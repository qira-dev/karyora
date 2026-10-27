package providers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// Egress controls (C3-18): every provider HTTP call goes through this
// guard. Rules enforced here, always:
//   - https only (http is allowed ONLY for an explicit local connection to
//     a loopback destination, C3-30);
//   - credentials never travel in URLs (userinfo and known credential query
//     parameters are refused at validation time);
//   - destinations resolve at dial time and every resolved IP is checked
//     against the prohibited-range set BEFORE the socket opens (DNS
//     rebinding cannot swap the address between check and dial);
//   - Remote Core refuses loopback, private, link-local (including cloud
//     metadata), CGNAT, benchmark, multicast, and unspecified ranges;
//   - redirects are disabled: a redirect is a protocol error, never a
//     followed hop (no credential forwarding across origins);
//   - hop-by-hop, Host-override, authorization-override, and internal-
//     routing headers are refused on connection-supplied headers;
//   - response bodies are size-capped; proxies are unsupported (the
//     environment proxy is deliberately NOT consulted — a proxy could
//     receive credentials, and proxy support needs its own recorded review).

// CoreMode distinguishes the local product Core from a remote deployment
// (C3-18/D-37 boundary): remote cores get the strict destination set.
type CoreMode string

const (
	CoreLocal  CoreMode = "local"
	CoreRemote CoreMode = "remote"
)

// EgressPolicy is the fail-closed egress configuration.
type EgressPolicy struct {
	Mode                  CoreMode
	MaxResponseBytes      int64
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	MaxRedirects          int // always 0: redirects are refused, never followed
}

// Default bounds (Proposed, pending the recorded owner review).
const (
	DefaultMaxResponseBytes      = 32 << 20
	DefaultMaxStreamLineBytes    = 1 << 20
	DefaultDialTimeout           = 10 * time.Second
	DefaultResponseHeaderTimeout = 60 * time.Second
)

func DefaultEgressPolicy(mode CoreMode) EgressPolicy {
	return EgressPolicy{
		Mode:                  mode,
		MaxResponseBytes:      DefaultMaxResponseBytes,
		DialTimeout:           DefaultDialTimeout,
		ResponseHeaderTimeout: DefaultResponseHeaderTimeout,
		MaxRedirects:          0,
	}
}

// credentialQueryParams are query parameter names that must never carry a
// value in a provider URL (C3-18: credentials never in URLs). A URL carrying
// any of these with a non-empty value is refused outright.
var credentialQueryParams = map[string]bool{
	"key": true, "api_key": true, "apikey": true, "api-key": true,
	"access_token": true, "token": true, "auth": true, "signature": true,
	"sig": true, "x-amz-signature": true, "client_secret": true,
	"password": true, "secret": true,
}

// ValidateDestinationURL validates the raw URL against the policy and
// returns the parsed URL. It is syntax-and-policy only; the IP check
// happens at dial time (ValidateDestinationURL alone cannot defeat DNS
// rebinding and does not claim to).
func ValidateDestinationURL(policy EgressPolicy, rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, dprovider.NewError(dprovider.ErrInvalidRequest, "egress", "destination URL does not parse")
	}
	switch u.Scheme {
	case "https":
		// always allowed
	case "http":
		if !(policy.Mode == CoreLocal && isLoopbackHost(u.Hostname())) {
			return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress",
				"plaintext http is allowed only for explicit local loopback connections")
		}
	default:
		return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress", "scheme "+u.Scheme+" is not allowed")
	}
	if u.User != nil {
		return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress",
			"credentials in URLs are refused (userinfo present)")
	}
	if u.Hostname() == "" {
		return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress", "destination host is empty")
	}
	for name, vals := range u.Query() {
		if credentialQueryParams[strings.ToLower(name)] {
			for _, v := range vals {
				if v != "" {
					return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress",
						"credentials in URLs are refused (query parameter "+name+")")
				}
			}
		}
	}
	if u.Fragment != "" {
		return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress", "fragments are not sent; refusing the URL")
	}
	return u, nil
}

func isLoopbackHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost"
}

// prohibitedRemote reports whether the IP is in a range a REMOTE core must
// never dial (C3-18): loopback, private, link-local (cloud metadata lives
// here), CGNAT, benchmark, multicast, link-local multicast, and the
// unspecified address. The control-plane metadata addresses are link-local
// and are already covered; fd00:ec2::254 (ULA metadata) is covered by the
// private ULA refusal.
func prohibitedRemote(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() ||
		ip.IsInterfaceLocalMulticast() || isCGNAT(ip) || isBenchmark(ip)
}

func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] < 128
}

func isBenchmark(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)
}

// localOnlyRefusal keeps the LOCAL core from being tricked into cloud
// metadata destinations even where other private ranges are its own
// network (a local core may reach the user's LAN; the metadata service is
// never a model endpoint).
func localOnlyRefusal(ip net.IP) bool {
	v4 := ip.To4()
	if v4 != nil && v4[0] == 169 && v4[1] == 254 && v4[2] == 254 {
		return true
	}
	// fd00:ec2::254 metadata (AWS IMDS over ULA)
	if ip.IsPrivate() && strings.HasPrefix(ip.String(), "fd00:ec2:") {
		return true
	}
	return false
}

// checkIP applies the mode's destination rules to one resolved address.
func checkIP(mode CoreMode, ip net.IP) error {
	if mode == CoreRemote && prohibitedRemote(ip) {
		return dprovider.NewError(dprovider.ErrEgressPolicy, "egress",
			"prohibited destination range refused for remote core ("+ip.String()+")")
	}
	if localOnlyRefusal(ip) {
		return dprovider.NewError(dprovider.ErrEgressPolicy, "egress",
			"cloud control-plane metadata address refused ("+ip.String()+")")
	}
	return nil
}

// guardedDialer resolves the host, checks EVERY resolved address, and dials
// the checked address directly — the address cannot change between check
// and connect (DNS rebinding defense, C3-18).
func guardedDialer(policy EgressPolicy) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, dprovider.NewError(dprovider.ErrEgressPolicy, "egress", "destination address does not split")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, dprovider.NewError(dprovider.ErrUnreachable, "egress", "destination did not resolve: "+err.Error())
		}
		if len(ips) == 0 {
			return nil, dprovider.NewError(dprovider.ErrUnreachable, "egress", "destination resolved to no addresses")
		}
		for _, ip := range ips {
			if err := checkIP(policy.Mode, ip.IP); err != nil {
				return nil, err
			}
		}
		d := net.Dialer{Timeout: policy.DialTimeout}
		// Dial the FIRST checked address: every address was verified above,
		// so dialing any of them cannot reach an unverified destination.
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
}

// NewTransport builds the guarded http.Transport for the policy. Redirects
// are refused at the client level via CheckRedirect in the http.Client;
// the transport carries the dial guard and TLS floor.
func NewTransport(policy EgressPolicy) *http.Transport {
	return &http.Transport{
		Proxy:                 nil, // proxies are unsupported in this slice (recorded limitation)
		DialContext:           guardedDialer(policy),
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: policy.ResponseHeaderTimeout,
		// ForceAttemptHTTP2 stays default; TLS 1.2+ is the floor.
	}
}

// errRedirectRefused is returned for every redirect: redirects are disabled
// for credentialed calls by default and revalidation-per-hop is NOT
// implemented in this slice (C3-18: "disabled by default").
var errRedirectRefused = errors.New("providers: redirects are disabled for provider egress (C3-18)")

// NewClient builds the guarded client: redirects refused, bounded header
// and body handling by the caller.
func NewClient(policy EgressPolicy) *http.Client {
	return &http.Client{
		Transport:     NewTransport(policy),
		CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirectRefused },
		Timeout:       0, // per-request contexts carry the deadline
	}
}

// forbiddenConnectionHeaders are the header names a CONNECTION record may
// never set (C3-18): the Host override, hop-by-hop machinery, the proxy/
// forwarded/internal-routing family, and every credential-bearing header
// the adapter itself must own. Comparison is case-insensitive.
var forbiddenConnectionHeaders = map[string]bool{
	"host": true, "content-length": true, "connection": true,
	"transfer-encoding": true, "upgrade": true, "te": true, "trailer": true,
	"keep-alive": true, "proxy-connection": true, "proxy-authorization": true,
	"proxy-authenticate": true, "expect": true, "authorization": true,
	"x-api-key": true, "x-goog-api-key": true,
	"x-forwarded-for": true, "x-forwarded-host": true, "x-forwarded-proto": true,
	"x-forwarded-port": true, "x-forwarded-server": true, "x-real-ip": true,
	"forwarded": true, "via": true, "cookie": true, "set-cookie": true,
}

// ValidateExtraHeader refuses a connection-supplied header that is
// forbidden or malformed. Header VALUE hygiene (no newlines) is enforced
// here too — header injection never passes (C3-18 evidence).
func ValidateExtraHeader(name, value string) error {
	if name == "" || strings.ContainsAny(name, " \t\r\n:") {
		return dprovider.NewError(dprovider.ErrInvalidRequest, "egress", "malformed header name")
	}
	if forbiddenConnectionHeaders[strings.ToLower(name)] {
		return dprovider.NewError(dprovider.ErrEgressPolicy, "egress",
			"connection header "+name+" is forbidden (Host/hop-by-hop/authorization-override/internal-routing)")
	}
	if strings.ContainsAny(value, "\r\n") || len(value) > 1024 {
		return dprovider.NewError(dprovider.ErrInvalidRequest, "egress", "header value for "+name+" is malformed or oversized")
	}
	return nil
}

// SetCredentialHeader writes the family's credential header from the
// ALREADY-RESOLVED secret bytes. This is the only place a credential is
// attached to a request; it never touches a URL, a log line, or an error
// string (C3-18/D-37).
func SetCredentialHeader(req *http.Request, headerName, secret string) {
	if headerName == "" || secret == "" {
		return
	}
	req.Header.Set(headerName, secret)
}

// cappedBody reads r up to policy.MaxResponseBytes and refuses larger
// responses (C3-18 request/response size rules).
func cappedBody(r io.Reader, policy EgressPolicy) ([]byte, error) {
	limited := io.LimitReader(r, policy.MaxResponseBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > policy.MaxResponseBytes {
		return nil, dprovider.NewError(dprovider.ErrResponseTooLarge, "egress",
			fmt.Sprintf("response exceeds the %d byte cap", policy.MaxResponseBytes))
	}
	return b, nil
}
