package redis

import "testing"

func TestOptionsFromPoolConfigParsesRedisURL(t *testing.T) {
	opts, err := optionsFromPoolConfig(PoolConfig{URL: "redis://user:secret@redis.example.com:6380/2"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Addr != "redis.example.com:6380" || opts.Username != "user" || opts.Password != "secret" || opts.DB != 2 {
		t.Fatalf("unexpected options: addr=%q username=%q db=%d", opts.Addr, opts.Username, opts.DB)
	}
	if opts.TLSConfig != nil {
		t.Fatal("redis:// must not enable TLS")
	}
}

func TestOptionsFromPoolConfigParsesRedissURL(t *testing.T) {
	opts, err := optionsFromPoolConfig(PoolConfig{URL: "rediss://redis.example.com:6380/1"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Addr != "redis.example.com:6380" || opts.DB != 1 || opts.TLSConfig == nil {
		t.Fatalf("unexpected rediss options: addr=%q db=%d tls=%v", opts.Addr, opts.DB, opts.TLSConfig != nil)
	}
}

func TestOptionsFromPoolConfigSupportsLegacyTCPAndTLSURLs(t *testing.T) {
	tcpOpts, err := optionsFromPoolConfig(PoolConfig{URL: "tcp://redis.example.com:6379"})
	if err != nil {
		t.Fatal(err)
	}
	if tcpOpts.Addr != "redis.example.com:6379" || tcpOpts.TLSConfig != nil {
		t.Fatalf("unexpected tcp options: addr=%q tls=%v", tcpOpts.Addr, tcpOpts.TLSConfig != nil)
	}

	tlsOpts, err := optionsFromPoolConfig(PoolConfig{URL: "tls://redis.example.com:6380"})
	if err != nil {
		t.Fatal(err)
	}
	if tlsOpts.Addr != "redis.example.com:6380" || tlsOpts.TLSConfig == nil || tlsOpts.TLSConfig.ServerName != "redis.example.com" {
		t.Fatalf("unexpected tls options: addr=%q server_name=%q", tlsOpts.Addr, tlsOpts.TLSConfig.ServerName)
	}
}

func TestOptionsFromPoolConfigRejectsMissingOrUnsupportedURL(t *testing.T) {
	for _, raw := range []string{"", "http://redis.example.com:6379"} {
		if _, err := optionsFromPoolConfig(PoolConfig{URL: raw}); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}
