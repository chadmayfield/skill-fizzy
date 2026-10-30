package config

import "testing"

func TestIsClusterService(t *testing.T) {
	for host, want := range map[string]bool{
		"fizzy.fizzy.svc":                true,
		"fizzy.fizzy.svc.cluster.local":  true,
		"FIZZY.FIZZY.SVC":                true,
		"fizzy.fizzy.svc.":               true,
		"fizzy.svc.cluster.local.":       true,
		".svc":                           false,
		"svc":                            false,
		"a..svc":                         false,
		"evil.svc.example.com":           false,
		"fizzy.svc.cluster.local.evil":   false,
		"fizzy.example.com":              false,
		"10.0.0.5":                       false,
		"fizzy.fizzy.svc.cluster.locals": false,
		"fe80::1%.svc":                   false,
		"fizzy_x.svc":                    false,
		"fi zzy.svc":                     false,
		"-fizzy.svc":                     false,
		"fizzy-1.ns-2.svc":               true,
	} {
		if got := isClusterService(host); got != want {
			t.Errorf("isClusterService(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestFromValuesClusterHTTP(t *testing.T) {
	if _, err := FromValues("http://fizzy.fizzy.svc", "1", "T", false); err == nil {
		t.Error("plain http to a service was accepted without the opt-in")
	}
	cfg, err := FromValues("http://fizzy.fizzy.svc.cluster.local:80/", "/1/", "T", true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://fizzy.fizzy.svc.cluster.local:80" || cfg.Account != "1" || cfg.LogPath != "" {
		t.Errorf("cfg = %+v", cfg)
	}
	for _, u := range []string{"http://user:pw@fizzy.svc", "http://fizzy.svc?x=1", "http://fizzy.svc#f"} {
		if _, err := FromValues(u, "1", "T", true); err == nil {
			t.Errorf("%s accepted", u)
		}
	}
}
