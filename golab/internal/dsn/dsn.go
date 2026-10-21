// Package dsn builds database connection strings with net/url. String
// concatenation breaks as soon as a password contains '@', ':' or '/'.
package dsn

import (
	"errors"
	"net"
	"net/url"
)

func Build(scheme, user, password, host, port, database string, params url.Values) (string, error) {
	if scheme == "" || host == "" || database == "" {
		return "", errors.New("scheme, host and database are required")
	}
	hostport := host
	if port != "" {
		hostport = net.JoinHostPort(host, port)
	}
	u := &url.URL{
		Scheme:   scheme,
		User:     url.UserPassword(user, password),
		Host:     hostport,
		Path:     "/" + database,
		RawQuery: params.Encode(),
	}
	return u.String(), nil
}

func Parse(raw string) (*url.URL, error) { return url.Parse(raw) }
