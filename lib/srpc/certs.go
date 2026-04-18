package srpc

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
	"time"

	"github.com/Cloud-Foundations/tricorder/go/tricorder"
	"github.com/Cloud-Foundations/tricorder/go/tricorder/units"
)

func getEarliestCert(tlsConfig *tls.Config) *x509.Certificate {
	if tlsConfig == nil {
		return nil
	}
	var earliestCert *x509.Certificate
	for _, cert := range tlsConfig.Certificates {
		if cert.Leaf != nil && !cert.Leaf.NotAfter.IsZero() {
			if earliestCert == nil {
				earliestCert = cert.Leaf
			} else if cert.Leaf.NotAfter.Before(earliestCert.NotAfter) {
				earliestCert = cert.Leaf
			}
		}
	}
	return earliestCert
}

func getEarliestCertExpiration(tlsConfig *tls.Config) time.Time {
	var earliest time.Time
	earliestCert := getEarliestCert(tlsConfig)
	if earliestCert == nil {
		return earliest
	}
	return earliestCert.NotAfter
}

func getEarliestExpiringCertActivation(tlsConfig *tls.Config) time.Time {
	var earliest time.Time
	earliestCert := getEarliestCert(tlsConfig)
	if earliestCert == nil {
		return earliest
	}
	return earliestCert.NotBefore
}

func setupCertExpirationMetric(once sync.Once, tlsConfig **tls.Config,
	metricsDir *tricorder.DirectorySpec) {
	if tlsConfig == nil {
		return
	}
	once.Do(func() {
		metricsDir.RegisterMetric("earliest-certificate-expiration",
			func() time.Time {
				return getEarliestCertExpiration(*tlsConfig)
			},
			units.None,
			"expiration time of the certificate which will expire the soonest")
		metricsDir.RegisterMetric("earliest-expiring-certificate-activation",
			func() time.Time {
				return getEarliestExpiringCertActivation(*tlsConfig)
			},
			units.None,
			"activation time of the certificate which will expire the soonest")
	})
}
