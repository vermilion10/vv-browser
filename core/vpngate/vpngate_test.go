package vpngate

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseAndInCountry(t *testing.T) {
	cfg := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	list := strings.Join([]string{
		"*vpn_servers",
		"#HostName,IP,Score,Ping,Speed,CountryLong,CountryShort,NumVpnSessions,Uptime,TotalUsers,TotalTraffic,LogType,Operator,Message,OpenVPN_ConfigData_Base64",
		"jp-low,1.1.1.1,100,20,1000,Japan,JP,1,1,1,1,2weeks,op,," + cfg("remote 1.1.1.1 443"),
		"kr,2.2.2.2,999,5,1000,Korea Republic of,KR,1,1,1,1,2weeks,op,," + cfg("remote 2.2.2.2 443"),
		"jp-high,3.3.3.3,500,10,2000,Japan,JP,1,1,1,1,2weeks,op,," + cfg("remote 3.3.3.3 443"),
		"broken,4.4.4.4,1,1,1,Japan,JP,1,1,1,1,2weeks,op,,!!notbase64!!",
		"*",
	}, "\r\n")

	servers, err := Parse(strings.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 3 {
		t.Fatalf("got %d servers, want 3 (rows with bad configs are skipped)", len(servers))
	}

	jp := InCountry(servers, "jp")
	if len(jp) != 2 || jp[0].HostName != "jp-high" || jp[1].HostName != "jp-low" {
		t.Fatalf("InCountry = %+v, want jp-high then jp-low", jp)
	}
	if string(jp[0].Config) != "remote 3.3.3.3 443" {
		t.Errorf("config not decoded: %q", jp[0].Config)
	}
}
