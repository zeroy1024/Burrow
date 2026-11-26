package profile

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RouteIdentity(route []GatewayHop, scope string, sshStep func(int) (Endpoint, string, string)) string {
	var identity strings.Builder
	fmt.Fprintf(&identity, "%q\n", scope)
	sshIndex := 0
	for _, hop := range route {
		fmt.Fprintf(&identity, "%q %q\n", hop.ID, hop.Kind)
		if hop.WG != nil {
			data, _ := json.Marshal(hop.WG)
			fmt.Fprintf(&identity, "%s\n", data)
			continue
		}
		if hop.Kind == "socks5" {
			s := hop.SOCKS
			fmt.Fprintf(&identity, "%q %d %q %t %q\n", s.Address, s.Port, s.Username, s.PasswordSaved, s.PasswordRevision)
			continue
		}
		h, secretID, fingerprint := sshStep(sshIndex)
		sshIndex++
		keepalive, healthCheck := 0, 0
		if h.SSH != nil {
			keepalive = h.SSH.KeepAlive
			healthCheck = h.SSH.HealthCheck
		}
		fmt.Fprintf(&identity, "%q %q %d %q %q %q %t %q %d %d %q %q\n",
			h.ID, h.Host, h.Port, h.User, h.Auth, h.KeyID,
			h.PasswordSaved, h.PasswordRevision, keepalive, healthCheck, secretID, fingerprint)
	}
	return identity.String()
}

func SameRoute(left, right *[]GatewayHop) bool {
	identity := func(value *[]GatewayHop) string {
		if value == nil {
			return RouteIdentity(nil, "", nil)
		}
		var steps []*Endpoint
		for _, hop := range *value {
			if hop.SSH != nil {
				steps = append(steps, hop.SSH)
			}
		}
		return RouteIdentity(*value, "", func(i int) (Endpoint, string, string) { return *steps[i], "", "" })
	}
	return identity(left) == identity(right)
}
