package route

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
)

const (
	ipAllowTag = "allow:ip"
	ipDenyTag  = "deny:ip"
)

// AccessDeniedHTTP checks rules on the target for HTTP proxy routes.
func (t *Target) AccessDeniedHTTP(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		log.Printf("[ERROR] Failed to get host from RemoteAddr %s: %s",
			r.RemoteAddr, err.Error())
		return false
	}

	// prefer xff header if set
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" && xff != host {
		host = xff
	}

	// currently only one function - more may be added in the future
	return t.denyByIP(net.ParseIP(host))
}

// AccessDeniedTCP checks rules on the target for TCP proxy routes.
func (t *Target) AccessDeniedTCP(c net.Conn) bool {
	// currently only one function - more may be added in the future
	return t.denyByIP(net.ParseIP(c.RemoteAddr().String()))
}

func (t *Target) denyByIP(ip net.IP) bool {
	if ip == nil || t.accessRules == nil {
		return false
	}

	// check allow (whitelist) first if it exists
	if _, ok := t.accessRules[ipAllowTag]; ok {
		var block *net.IPNet
		for _, x := range t.accessRules[ipAllowTag] {
			if block, ok = x.(*net.IPNet); !ok {
				log.Print("[ERROR] failed to assert ip block while checking allow rule for ", t.Service)
				continue
			}
			if block.Contains(ip) {
				// specific allow matched - allow this request
				return false
			}
		}
		// we checked all the blocks - deny this request
		return true
	}

	// still going - check deny (blacklist) if it exists
	if _, ok := t.accessRules[ipDenyTag]; ok {
		var block *net.IPNet
		for _, x := range t.accessRules[ipDenyTag] {
			if block, ok = x.(*net.IPNet); !ok {
				log.Print("[INFO] failed to assert ip block while checking deny rule for ", t.Service)
				continue
			}
			if block.Contains(ip) {
				// specific deny matched - deny this request
				return true
			}
		}
	}

	// default - do not deny
	return false
}

func (t *Target) parseAccessRule(allowDeny string) error {
	var accessTag string
	var temps []string

	// init rules if needed
	if t.accessRules == nil {
		t.accessRules = make(map[string][]interface{})
	}

	// loop over rule elements
	for _, c := range strings.Split(t.Opts[allowDeny], ",") {
		if temps = strings.SplitN(c, ":", 2); len(temps) != 2 {
			return fmt.Errorf("invalid access item, expected <type>:<data>, got %s", temps)
		}
		accessTag = allowDeny + ":" + strings.ToLower(temps[0])
		switch accessTag {
		case ipAllowTag, ipDenyTag:
			_, net, err := net.ParseCIDR(strings.TrimSpace(temps[1]))
			if err != nil {
				return fmt.Errorf("failed to parse CIDR %s with error: %s",
					c, err.Error())
			}
			// add element to rule map
			t.accessRules[accessTag] = append(t.accessRules[accessTag], net)
		default:
			return fmt.Errorf("unknown access item type: %s", temps[0])
		}
	}
	return nil
}

func (t *Target) processAccessRules() error {
	if t.Opts["allow"] != "" && t.Opts["deny"] != "" {
		return errors.New("specifying allow and deny on the same route is not supported")
	}

	for _, allowDeny := range []string{"allow", "deny"} {
		if t.Opts[allowDeny] != "" {
			if err := t.parseAccessRule(allowDeny); err != nil {
				return err
			}
		}
	}
	return nil
}
