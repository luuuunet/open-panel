package appstore

import "testing"

func TestStackFallbackSupported(t *testing.T) {
	want := []string{
		"nginx", "redis", "postgresql", "mongodb", "docker", "certbot",
		"memcached", "fail2ban", "apache", "openresty", "pureftpd",
	}
	for _, key := range want {
		if !stackFallbackSupported(key) {
			t.Fatalf("stackFallbackSupported(%q) = false, want true", key)
		}
	}
	if !stackFallbackSupported("php83") {
		t.Fatal("php83 should support stack fallback")
	}
	if stackFallbackSupported("wordpress-app") {
		t.Fatal("docker apps should not use stack fallback")
	}
	if stackFallbackComponent("mysql") != "mariadb" {
		t.Fatalf("mysql component = %q, want mariadb", stackFallbackComponent("mysql"))
	}
}
