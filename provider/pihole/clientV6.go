/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pihole

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	extdnshttp "sigs.k8s.io/external-dns/pkg/http"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider"
)

const (
	contentTypeJSON = "application/json"
	apiAuthPath     = "/api/auth"
	apiConfigDNS    = "/api/config/dns"
)

// piholeClient implements the piholeAPI.
type piholeClientV6 struct {
	cfg        PiholeConfig
	httpClient *http.Client
	token      string
}

// newPiholeClient creates a new Pihole API V6 client.
func newPiholeClientV6(cfg PiholeConfig) (piholeAPI, error) {
	if cfg.Server == "" {
		return nil, ErrNoPiholeServer
	}

	// Setup an HTTP client
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.TLSInsecureSkipVerify,
			},
		},
	}

	cl := extdnshttp.NewInstrumentedClient(httpClient)

	p := &piholeClientV6{
		cfg:        cfg,
		httpClient: cl,
	}

	if cfg.Password != "" {
		if err := p.retrieveNewToken(context.Background()); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func (p *piholeClientV6) getConfigValue(ctx context.Context, rtype string) ([]string, error) {
	apiUrl, err := p.urlForRecordType(rtype)
	if err != nil {
		return nil, err
	}

	log.Debugf("Listing %s records from %s", rtype, apiUrl)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiUrl, nil)
	if err != nil {
		return nil, err
	}

	jRes, err := p.do(req)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var apiResponse ApiRecordsResponse
	if err := json.Unmarshal(jRes, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal error response: %w", err)
	}

	// Pi-Hole does not allow for a record to have multiple targets.
	var results []string
	if endpoint.RecordTypeCNAME == rtype {
		results = apiResponse.Config.DNS.CnameRecords
	} else {
		results = apiResponse.Config.DNS.Hosts
	}

	return results, nil
}

/**
 * isValidIPv4 checks if the given IP address is a valid IPv4 address.
 * It returns true if the IP address is valid, false otherwise.
 * If the IP address is in IPv6 format, it will return false.
 */
func isValidIPv4(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.Is4()
}

/**
 * isValidIPv6 checks if the given IP address is a valid IPv6 address.
 * It returns true if the IP address is valid, false otherwise.
 * If the IP address is in IPv6 with dual format y:y:y:y:y:y:x.x.x.x. , it will return true.
 */
func isValidIPv6(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.Is6()
}

func (p *piholeClientV6) listRecords(ctx context.Context, rtype string) ([]*endpoint.Endpoint, error) {
	results, err := p.getConfigValue(ctx, rtype)
	if err != nil {
		return nil, err
	}

	endpoints := make(map[string]*endpoint.Endpoint)

	for _, rec := range results {
		recs := strings.FieldsFunc(rec, func(r rune) bool {
			return r == ' ' || r == ','
		})
		if len(recs) < 2 {
			log.Warnf("skipping record %s: invalid format received from PiHole", rec)
			continue
		}
		var DNSName, Target string
		var Ttl = endpoint.TTL(0)
		// A/AAAA record format is target(IP) DNSName
		DNSName, Target = recs[1], recs[0]
		switch rtype {
		case endpoint.RecordTypeA:
			// PiHole return A and AAAA records. Filter to only keep the A records
			if !isValidIPv4(Target) {
				continue
			}
		case endpoint.RecordTypeAAAA:
			// PiHole return A and AAAA records. Filter to only keep the AAAA records
			if !isValidIPv6(Target) {
				continue
			}
		case endpoint.RecordTypeCNAME:
			// PiHole return only CNAME records.
			// CNAME format is DNSName,target, ttl?
			DNSName, Target = recs[0], recs[1]
			if len(recs) == 3 { // TTL is present
				// Parse string to int64 first
				if ttlInt, err := strconv.ParseInt(recs[2], 10, 64); err == nil {
					Ttl = endpoint.TTL(ttlInt)
				} else {
					log.Warnf("failed to parse TTL value received from PiHole '%s': %v; using a TTL of %d", recs[2], err, Ttl)
				}
			}
		}

		ep := endpoint.NewEndpointWithTTL(DNSName, rtype, Ttl, Target)

		if oldEp, ok := endpoints[DNSName]; ok {
			ep.Targets = append(oldEp.Targets, Target)
		}

		endpoints[DNSName] = ep
	}

	out := make([]*endpoint.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		out = append(out, ep)
	}
	return out, nil
}

func (p *piholeClientV6) createRecord(ctx context.Context, ep *endpoint.Endpoint) error {
	return p.apply(ctx, http.MethodPut, ep)
}

func (p *piholeClientV6) deleteRecord(ctx context.Context, ep *endpoint.Endpoint) error {
	return p.apply(ctx, http.MethodDelete, ep)
}

func (p *piholeClientV6) aRecordsScript() string {
	return fmt.Sprintf("%s"+apiConfigDNS+"/hosts", p.cfg.Server)
}

func (p *piholeClientV6) cnameRecordsScript() string {
	return fmt.Sprintf("%s"+apiConfigDNS+"/cnameRecords", p.cfg.Server)
}

func (p *piholeClientV6) urlForRecordType(rtype string) (string, error) {
	switch rtype {
	case endpoint.RecordTypeA, endpoint.RecordTypeAAAA:
		return p.aRecordsScript(), nil
	case endpoint.RecordTypeCNAME:
		return p.cnameRecordsScript(), nil
	default:
		return "", fmt.Errorf("unsupported record type: %s", rtype)
	}
}

// ApiAuthResponse Define a struct to match the JSON response /auth/app structure
type ApiAuthResponse struct {
	Session struct {
		Valid    bool   `json:"valid"`
		TOTP     bool   `json:"totp"`
		SID      string `json:"sid"`
		CSRF     string `json:"csrf"`
		Validity int    `json:"validity"`
		Message  string `json:"message"`
	} `json:"session"`
	Took float64 `json:"took"`
}

// ApiErrorResponse Define struct to match the JSON structure
type ApiErrorResponse struct {
	Error struct {
		Key     string `json:"key"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
	Took float64 `json:"took"`
}

// ApiRecordsResponse Define struct to match JSON structure
type ApiRecordsResponse struct {
	Config struct {
		DNS struct {
			Hosts        []string `json:"hosts"`
			CnameRecords []string `json:"cnameRecords"`
		} `json:"dns"`
	} `json:"config"`
	Took float64 `json:"took"`
}

func (p *piholeClientV6) generateApiUrl(baseUrl, params string) string {
	return fmt.Sprintf("%s/%s", baseUrl, url.PathEscape(params))
}

func (p *piholeClientV6) apply(ctx context.Context, action string, ep *endpoint.Endpoint) error {
	if !p.cfg.DomainFilter.Match(ep.DNSName) {
		log.Debugf("Skipping : %s %s that does not match domain filter", action, ep.DNSName)
		return nil
	}
	apiUrl, err := p.urlForRecordType(ep.RecordType)
	if err != nil {
		log.Warnf("Skipping : unsupported endpoint %s %s %v", ep.DNSName, ep.RecordType, ep.Targets)
		return nil
	}

	if len(ep.Targets) == 0 {
		log.Infof("Skipping : missing targets  %s %s %s", action, ep.DNSName, ep.RecordType)
		return nil
	}

	// Get the current record
	if strings.Contains(ep.DNSName, "*") {
		return provider.NewSoftError(errors.New("UNSUPPORTED: Pihole DNS names cannot return wildcard"))
	}

	if ep.RecordType == endpoint.RecordTypeCNAME && len(ep.Targets) > 1 {
		return provider.NewSoftError(errors.New("UNSUPPORTED: Pihole CNAME records cannot have multiple targets"))
	}

	for _, target := range ep.Targets {
		if p.cfg.DryRun {
			log.Infof("DRY RUN: %s %s IN %s -> %s", action, ep.DNSName, ep.RecordType, target)
			continue
		}

		log.Infof("%s %s IN %s -> %s", action, ep.DNSName, ep.RecordType, target)

		targetApiUrl := apiUrl

		switch ep.RecordType {
		case endpoint.RecordTypeA, endpoint.RecordTypeAAAA:
			targetApiUrl = p.generateApiUrl(targetApiUrl, fmt.Sprintf("%s %s", target, ep.DNSName))
		case endpoint.RecordTypeCNAME:
			if ep.RecordTTL.IsConfigured() {
				targetApiUrl = p.generateApiUrl(targetApiUrl, fmt.Sprintf("%s,%s,%d", ep.DNSName, target, ep.RecordTTL))
			} else {
				targetApiUrl = p.generateApiUrl(targetApiUrl, fmt.Sprintf("%s,%s", ep.DNSName, target))
			}
		}
		req, err := http.NewRequestWithContext(ctx, action, targetApiUrl, nil)
		if err != nil {
			return err
		}

		_, err = p.do(req)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *piholeClientV6) retrieveNewToken(ctx context.Context) error {
	if p.cfg.Password == "" {
		return nil
	}

	apiUrl := fmt.Sprintf("%s"+apiAuthPath, p.cfg.Server)
	log.Debugf("Fetching new token from %s", apiUrl)

	// Define the JSON payload
	jsonData := []byte(`{"password":"` + p.cfg.Password + `"}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	jRes, err := p.do(req)
	if err != nil {
		return err
	}

	// Parse JSON response
	var apiResponse ApiAuthResponse
	if err := json.Unmarshal(jRes, &apiResponse); err != nil {
		log.Errorf("Auth Query : failed to unmarshal error response: %v", err)
	} else {
		// Set the token
		if apiResponse.Session.SID != "" {
			p.token = apiResponse.Session.SID
		}
	}
	return err
}

func (p *piholeClientV6) checkTokenValidity(ctx context.Context) (bool, error) {
	if p.token == "" {
		return false, nil
	}

	apiUrl := fmt.Sprintf("%s"+apiAuthPath, p.cfg.Server)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiUrl, nil)
	if err != nil {
		return false, nil
	}
	req.Header.Add("content-type", contentTypeJSON)
	if p.token != "" {
		req.Header.Add("X-FTL-SID", p.token)
	}
	res, err := p.httpClient.Do(req)
	if err != nil {
		return false, err
	}

	jRes, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return false, err
	}

	// Parse JSON response
	var apiResponse ApiAuthResponse
	if err := json.Unmarshal(jRes, &apiResponse); err != nil {
		return false, fmt.Errorf("failed to unmarshal error response: %w", err)
	}
	return apiResponse.Session.Valid, nil
}

func (p *piholeClientV6) do(req *http.Request) ([]byte, error) {
	req.Header.Add("content-type", contentTypeJSON)
	if p.token != "" {
		req.Header.Add("X-FTL-SID", p.token)
	}
	res, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	jRes, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK &&
		res.StatusCode != http.StatusCreated &&
		res.StatusCode != http.StatusNoContent {
		// Parse JSON response
		var apiError ApiErrorResponse
		if err := json.Unmarshal(jRes, &apiError); err != nil {
			return nil, fmt.Errorf("failed to unmarshal error response: %w", err)
		}
		// Ignore if the entry already exists when adding a record
		if strings.Contains(apiError.Error.Message, "Item already present") {
			return jRes, nil
		}
		// Ignore if the entry does not exist when deleting a record
		if res.StatusCode == http.StatusNotFound && req.Method == http.MethodDelete {
			return jRes, nil
		}
		if log.IsLevelEnabled(log.DebugLevel) {
			log.Debugf("Error on request %s", req.URL)
			if req.Body != nil {
				log.Debugf("Body of the request %s", req.Body)
			}
		}

		if res.StatusCode == http.StatusUnauthorized && p.token != "" {
			tryCount := 1
			maxRetries := 3
			// Try to fetch a new token and redo the request.
			for tryCount <= maxRetries {
				valid, err := p.checkTokenValidity(req.Context())
				if err != nil {
					return nil, err
				}
				if !valid {
					log.Debugf("Pihole token has expired, fetching a new one. Try (%d/%d)", tryCount, maxRetries)
					if err := p.retrieveNewToken(req.Context()); err != nil {
						return nil, err
					}
					tryCount++
					continue
				}
				break
			}
			if tryCount > maxRetries {
				return nil, errors.New("max tries reached for token renewal")
			}
			return p.do(req)
		}
		return nil, fmt.Errorf("received %d status code from request: [%s] %s (%s) - %fs", res.StatusCode, apiError.Error.Key, apiError.Error.Message, apiError.Error.Hint, apiError.Took)
	}
	return jRes, nil
}
