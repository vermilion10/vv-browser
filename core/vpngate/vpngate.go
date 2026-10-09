// Package vpngate fetches and parses the public VPN Gate relay server list.
package vpngate

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// APIURL serves the server list as CSV, one row per relay, each carrying a
// base64-encoded OpenVPN config.
const APIURL = "https://www.vpngate.net/api/iphone/"

// Server is one VPN Gate relay.
type Server struct {
	HostName string
	IP       string
	Score    int64
	PingMS   int64
	SpeedBps int64
	Country  string // ISO 3166 alpha-2, e.g. "JP"
	Sessions int64
	Config   []byte // OpenVPN config (.ovpn contents)
}

// Fetch downloads and parses the current server list.
func Fetch(ctx context.Context, client *http.Client) ([]Server, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vpngate: unexpected status %s", resp.Status)
	}
	return Parse(resp.Body)
}

// Parse reads the API's CSV format. The list is wrapped in a "*vpn_servers"
// line and a trailing "*" line, and the header row starts with "#HostName".
func Parse(r io.Reader) ([]Server, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	var servers []Server
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("vpngate: %w", err)
		}
		if len(rec) != 15 || strings.HasPrefix(rec[0], "#") || strings.HasPrefix(rec[0], "*") {
			continue
		}
		cfg, err := base64.StdEncoding.DecodeString(rec[14])
		if err != nil || len(cfg) == 0 {
			continue
		}
		servers = append(servers, Server{
			HostName: rec[0],
			IP:       rec[1],
			Score:    atoi(rec[2]),
			PingMS:   atoi(rec[3]),
			SpeedBps: atoi(rec[4]),
			Country:  rec[6],
			Sessions: atoi(rec[7]),
			Config:   cfg,
		})
	}
	return servers, nil
}

// InCountry returns the servers in the given country, best score first.
func InCountry(servers []Server, country string) []Server {
	var out []Server
	for _, s := range servers {
		if strings.EqualFold(s.Country, country) {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func atoi(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
