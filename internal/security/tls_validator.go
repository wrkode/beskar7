package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// TLSValidator provides TLS certificate validation functionality
type TLSValidator struct {
	CustomCAs       *x509.CertPool
	Timeout         time.Duration
	AllowSelfSigned bool
}

// NewTLSValidator creates a new TLS validator with default settings
func NewTLSValidator() *TLSValidator {
	return &TLSValidator{
		Timeout:         30 * time.Second,
		AllowSelfSigned: false,
	}
}

// ValidationResult contains the result of TLS validation
type ValidationResult struct {
	Valid        bool
	Warnings     []string
	Errors       []string
	Certificate  *x509.Certificate
	CertChain    []*x509.Certificate
	Expiry       time.Time
	IsSelfSigned bool
	SANs         []string
}

// ValidateTLSEndpoint validates the TLS configuration of a given endpoint
func (v *TLSValidator) ValidateTLSEndpoint(ctx context.Context, endpoint string) (*ValidationResult, error) {
	result := &ValidationResult{
		Warnings: make([]string, 0),
		Errors:   make([]string, 0),
	}

	// Parse the endpoint URL
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Invalid URL: %v", err))
		return result, nil
	}

	// Only validate HTTPS endpoints
	if parsedURL.Scheme != "https" {
		result.Errors = append(result.Errors, "Only HTTPS endpoints can be validated")
		return result, nil
	}

	host := parsedURL.Hostname()
	port := parsedURL.Port()
	if port == "" {
		port = "443" // Default HTTPS port
	}

	address := net.JoinHostPort(host, port)

	// Create a dialer with timeout
	dialer := &net.Dialer{
		Timeout: v.Timeout,
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true, // We handle verification manually
		RootCAs:            v.CustomCAs,
	}

	// Connect and get certificate info
	conn, err := tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect: %v", err))
		return result, nil
	}
	defer conn.Close()

	// Get the peer certificate chain
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		result.Errors = append(result.Errors, "No certificates found")
		return result, nil
	}

	cert := state.PeerCertificates[0]
	result.Certificate = cert
	result.CertChain = state.PeerCertificates
	result.Expiry = cert.NotAfter
	result.SANs = cert.DNSNames

	// Check if certificate is self-signed
	result.IsSelfSigned = v.isSelfSigned(cert)

	// Perform validation checks
	v.validateCertificate(result, cert, host)
	v.validateCertificateChain(result, state.PeerCertificates)
	v.validateExpiry(result, cert)
	v.validateHostname(result, cert, host)

	// Determine overall validity
	result.Valid = len(result.Errors) == 0

	return result, nil
}

// isSelfSigned checks if a certificate is self-signed
func (v *TLSValidator) isSelfSigned(cert *x509.Certificate) bool {
	return cert.Issuer.String() == cert.Subject.String()
}

// validateCertificate performs basic certificate validation
func (v *TLSValidator) validateCertificate(result *ValidationResult, cert *x509.Certificate, hostname string) {
	now := time.Now()

	// Check if certificate is valid for current time
	if now.Before(cert.NotBefore) {
		result.Errors = append(result.Errors, "Certificate is not yet valid")
	}

	if now.After(cert.NotAfter) {
		result.Errors = append(result.Errors, "Certificate has expired")
	}

	// Check if self-signed and whether it's allowed
	if result.IsSelfSigned && !v.AllowSelfSigned {
		result.Errors = append(result.Errors, "Self-signed certificates are not allowed")
	} else if result.IsSelfSigned {
		result.Warnings = append(result.Warnings, "Certificate is self-signed")
	}

	// Check key usage
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		result.Warnings = append(result.Warnings, "Certificate lacks digital signature key usage")
	}

	if cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 && cert.KeyUsage&x509.KeyUsageKeyAgreement == 0 {
		result.Warnings = append(result.Warnings, "Certificate lacks key encipherment or key agreement usage")
	}

	// Check extended key usage
	validExtKeyUsage := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			validExtKeyUsage = true
			break
		}
	}
	if !validExtKeyUsage {
		result.Warnings = append(result.Warnings, "Certificate lacks server authentication extended key usage")
	}
}

// validateCertificateChain validates the certificate chain
func (v *TLSValidator) validateCertificateChain(result *ValidationResult, chain []*x509.Certificate) {
	if len(chain) == 1 && !result.IsSelfSigned {
		result.Warnings = append(result.Warnings, "Certificate chain contains only leaf certificate (missing intermediate CAs)")
	}

	// Validate each certificate in the chain
	for i, cert := range chain {
		if cert.IsCA && i == 0 {
			result.Errors = append(result.Errors, "Leaf certificate incorrectly marked as CA")
		}
		if !cert.IsCA && i > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Intermediate certificate at position %d not marked as CA", i))
		}
	}
}

// validateExpiry checks certificate expiry and warns about upcoming expiration
func (v *TLSValidator) validateExpiry(result *ValidationResult, cert *x509.Certificate) {
	now := time.Now()

	// Warn if certificate expires within 30 days
	if cert.NotAfter.Sub(now) < 30*24*time.Hour && cert.NotAfter.After(now) {
		result.Warnings = append(result.Warnings, "Certificate expires within 30 days")
	}

	// Warn if certificate expires within 7 days
	if cert.NotAfter.Sub(now) < 7*24*time.Hour && cert.NotAfter.After(now) {
		result.Warnings = append(result.Warnings, "Certificate expires within 7 days")
	}
}

// validateHostname validates that the certificate is valid for the given hostname
func (v *TLSValidator) validateHostname(result *ValidationResult, cert *x509.Certificate, hostname string) {
	err := cert.VerifyHostname(hostname)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Certificate not valid for hostname %s: %v", hostname, err))

		// Provide suggestions
		if len(cert.DNSNames) > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Certificate is valid for: %s", strings.Join(cert.DNSNames, ", ")))
		}

		if len(cert.IPAddresses) > 0 {
			ips := make([]string, len(cert.IPAddresses))
			for i, ip := range cert.IPAddresses {
				ips[i] = ip.String()
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("Certificate is valid for IPs: %s", strings.Join(ips, ", ")))
		}
	}
}

// GetCertificateInfo returns human-readable certificate information
func (v *TLSValidator) GetCertificateInfo(cert *x509.Certificate) map[string]interface{} {
	info := map[string]interface{}{
		"subject":       cert.Subject.String(),
		"issuer":        cert.Issuer.String(),
		"serial":        cert.SerialNumber.String(),
		"not_before":    cert.NotBefore.Format(time.RFC3339),
		"not_after":     cert.NotAfter.Format(time.RFC3339),
		"dns_names":     cert.DNSNames,
		"ip_addresses":  cert.IPAddresses,
		"is_ca":         cert.IsCA,
		"key_usage":     v.getKeyUsageStrings(cert.KeyUsage),
		"ext_key_usage": v.getExtKeyUsageStrings(cert.ExtKeyUsage),
	}

	return info
}

// getKeyUsageStrings converts key usage flags to human-readable strings
func (v *TLSValidator) getKeyUsageStrings(usage x509.KeyUsage) []string {
	var usages []string

	if usage&x509.KeyUsageDigitalSignature != 0 {
		usages = append(usages, "Digital Signature")
	}
	if usage&x509.KeyUsageContentCommitment != 0 {
		usages = append(usages, "Content Commitment")
	}
	if usage&x509.KeyUsageKeyEncipherment != 0 {
		usages = append(usages, "Key Encipherment")
	}
	if usage&x509.KeyUsageDataEncipherment != 0 {
		usages = append(usages, "Data Encipherment")
	}
	if usage&x509.KeyUsageKeyAgreement != 0 {
		usages = append(usages, "Key Agreement")
	}
	if usage&x509.KeyUsageCertSign != 0 {
		usages = append(usages, "Certificate Sign")
	}
	if usage&x509.KeyUsageCRLSign != 0 {
		usages = append(usages, "CRL Sign")
	}
	if usage&x509.KeyUsageEncipherOnly != 0 {
		usages = append(usages, "Encipher Only")
	}
	if usage&x509.KeyUsageDecipherOnly != 0 {
		usages = append(usages, "Decipher Only")
	}

	return usages
}

// getExtKeyUsageStrings converts extended key usage to human-readable strings
func (v *TLSValidator) getExtKeyUsageStrings(usage []x509.ExtKeyUsage) []string {
	var usages []string

	for _, u := range usage {
		switch u {
		case x509.ExtKeyUsageServerAuth:
			usages = append(usages, "Server Authentication")
		case x509.ExtKeyUsageClientAuth:
			usages = append(usages, "Client Authentication")
		case x509.ExtKeyUsageCodeSigning:
			usages = append(usages, "Code Signing")
		case x509.ExtKeyUsageEmailProtection:
			usages = append(usages, "Email Protection")
		case x509.ExtKeyUsageTimeStamping:
			usages = append(usages, "Time Stamping")
		case x509.ExtKeyUsageOCSPSigning:
			usages = append(usages, "OCSP Signing")
		default:
			usages = append(usages, fmt.Sprintf("Unknown (%v)", u))
		}
	}

	return usages
}
