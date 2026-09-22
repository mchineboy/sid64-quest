package config

import (
	"net/url"
	"testing"
)

func TestPostgresConnectionStringEncodesCredentialsAndDatabase(t *testing.T) {
	for _, password := range []string{"", "space quote' slash/ dollar$ equals= question?"} {
		c := PostgreSQLConfig{Host: "::1", Port: 5432, Username: "user name", Password: password, Database: "isolated-db", SSLMode: "disable"}
		u, err := url.Parse(c.ConnectionString())
		if err != nil {
			t.Fatal(err)
		}
		got, _ := u.User.Password()
		if got != password || u.User.Username() != c.Username || u.Path != "/isolated-db" || u.Host != "[::1]:5432" {
			t.Fatalf("connection fields did not round trip")
		}
	}
}
