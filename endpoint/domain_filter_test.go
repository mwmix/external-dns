/*
Copyright 2017 The Kubernetes Authors.

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

package endpoint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type domainFilterTest struct {
	domainFilter          []string
	exclusions            []string
	domains               []string
	expected              bool
	expectedSerialization map[string][]string
}

type regexDomainFilterTest struct {
	regex                 *regexp.Regexp
	regexExclusion        *regexp.Regexp
	domains               []string
	expected              bool
	expectedSerialization map[string]string
}

var domainFilterTests = []domainFilterTest{
	{
		[]string{"google.com.", "exaring.de", "inovex.de"},
		[]string{},
		[]string{"google.com", "exaring.de", "inovex.de"},
		true,
		map[string][]string{
			"include": {"exaring.de", "google.com", "inovex.de"},
		},
	},
	{
		[]string{"google.com.", "exaring.de", "inovex.de"},
		[]string{},
		[]string{"google.com", "exaring.de", "inovex.de"},
		true,
		map[string][]string{
			"include": {"exaring.de", "google.com", "inovex.de"},
		},
	},
	{
		[]string{"google.com.", "exaring.de.", "inovex.de"},
		[]string{},
		[]string{"google.com", "exaring.de", "inovex.de"},
		true,
		map[string][]string{
			"include": {"exaring.de", "google.com", "inovex.de"},
		},
	},
	{
		[]string{"foo.org.      "},
		[]string{},
		[]string{"foo.org"},
		true,
		map[string][]string{
			"include": {"foo.org"},
		},
	},
	{
		[]string{"   foo.org"},
		[]string{},
		[]string{"foo.org"},
		true,
		map[string][]string{
			"include": {"foo.org"},
		},
	},
	{
		[]string{"foo.org."},
		[]string{},
		[]string{"foo.org"},
		true,
		map[string][]string{
			"include": {"foo.org"},
		},
	},
	{
		[]string{"foo.org."},
		[]string{},
		[]string{"baz.org"},
		false,
		map[string][]string{
			"include": {"foo.org"},
		},
	},
	{
		[]string{"baz.foo.org."},
		[]string{},
		[]string{"foo.org"},
		false,
		map[string][]string{
			"include": {"baz.foo.org"},
		},
	},
	{
		[]string{"", "foo.org."},
		[]string{},
		[]string{"foo.org"},
		true,
		map[string][]string{
			"include": {"foo.org"},
		},
	},
	{
		[]string{"", "foo.org."},
		[]string{},
		[]string{},
		true,
		map[string][]string{
			"include": {"foo.org"},
		},
	},
	{
		[]string{""},
		[]string{},
		[]string{"foo.org"},
		true,
		map[string][]string{},
	},
	{
		[]string{""},
		[]string{},
		[]string{},
		true,
		map[string][]string{},
	},
	{
		[]string{" "},
		[]string{},
		[]string{},
		true,
		map[string][]string{},
	},
	{
		[]string{"bar.sub.example.org"},
		[]string{},
		[]string{"foo.bar.sub.example.org"},
		true,
		map[string][]string{
			"include": {"bar.sub.example.org"},
		},
	},
	{
		[]string{"example.org"},
		[]string{},
		[]string{"anexample.org", "test.anexample.org"},
		false,
		map[string][]string{
			"include": {"example.org"},
		},
	},
	{
		[]string{".example.org"},
		[]string{},
		[]string{"anexample.org", "test.anexample.org"},
		false,
		map[string][]string{
			"include": {".example.org"},
		},
	},
	{
		[]string{".example.org"},
		[]string{},
		[]string{"example.org"},
		false,
		map[string][]string{
			"include": {".example.org"},
		},
	},
	{
		[]string{".example.org"},
		[]string{},
		[]string{"test.example.org"},
		true,
		map[string][]string{
			"include": {".example.org"},
		},
	},
	{
		[]string{"anexample.org"},
		[]string{},
		[]string{"example.org", "test.example.org"},
		false,
		map[string][]string{
			"include": {"anexample.org"},
		},
	},
	{
		[]string{".org"},
		[]string{},
		[]string{"example.org", "test.example.org", "foo.test.example.org"},
		true,
		map[string][]string{
			"include": {".org"},
		},
	},
	{
		[]string{"example.org"},
		[]string{"api.example.org"},
		[]string{"example.org", "test.example.org", "foo.test.example.org"},
		true,
		map[string][]string{
			"include": {"example.org"},
			"exclude": {"api.example.org"},
		},
	},
	{
		[]string{"example.org"},
		[]string{"api.example.org"},
		[]string{"foo.api.example.org", "api.example.org"},
		false,
		map[string][]string{
			"include": {"example.org"},
			"exclude": {"api.example.org"},
		},
	},
	{
		[]string{"   example.org. "},
		[]string{"   .api.example.org    "},
		[]string{"foo.api.example.org", "bar.baz.api.example.org."},
		false,
		map[string][]string{
			"include": {"example.org"},
			"exclude": {".api.example.org"},
		},
	},
	{
		[]string{"æøå.org"},
		[]string{"api.æøå.org"},
		[]string{"foo.api.æøå.org", "api.æøå.org"},
		false,
		map[string][]string{
			"include": {"æøå.org"},
			"exclude": {"api.æøå.org"},
		},
	},
	{
		[]string{"   æøå.org. "},
		[]string{"   .api.æøå.org    "},
		[]string{"foo.api.æøå.org", "bar.baz.api.æøå.org."},
		false,
		map[string][]string{
			"include": {"æøå.org"},
			"exclude": {".api.æøå.org"},
		},
	},
	{
		[]string{"example.org."},
		[]string{"api.example.org"},
		[]string{"dev-api.example.org", "qa-api.example.org"},
		true,
		map[string][]string{
			"include": {"example.org"},
			"exclude": {"api.example.org"},
		},
	},
	{
		[]string{"example.org."},
		[]string{"api.example.org"},
		[]string{"dev.api.example.org", "qa.api.example.org"},
		false,
		map[string][]string{
			"include": {"example.org"},
			"exclude": {"api.example.org"},
		},
	},
	{
		[]string{"example.org", "api.example.org"},
		[]string{"internal.api.example.org"},
		[]string{"foo.api.example.org"},
		true,
		map[string][]string{
			"include": {"api.example.org", "example.org"},
			"exclude": {"internal.api.example.org"},
		},
	},
	{
		[]string{"example.org", "api.example.org"},
		[]string{"internal.api.example.org"},
		[]string{"foo.internal.api.example.org"},
		false,
		map[string][]string{
			"include": {"api.example.org", "example.org"},
			"exclude": {"internal.api.example.org"},
		},
	},
	{
		[]string{"eXaMPle.ORG", "API.example.ORG"},
		[]string{"Foo-Bar.Example.Org"},
		[]string{"FoOoo.Api.Example.Org"},
		true,
		map[string][]string{
			"include": {"api.example.org", "example.org"},
			"exclude": {"foo-bar.example.org"},
		},
	},
	{
		[]string{"sTOnks📈.ORG", "API.xn--StonkS-u354e.ORG"},
		[]string{"Foo-Bar.stoNks📈.Org"},
		[]string{"FoOoo.Api.Stonks📈.Org"},
		true,
		map[string][]string{
			"include": {"api.stonks📈.org", "stonks📈.org"},
			"exclude": {"foo-bar.stonks📈.org"},
		},
	},
	{
		[]string{"eXaMPle.ORG", "API.example.ORG"},
		[]string{"api.example.org"},
		[]string{"foobar.Example.Org"},
		true,
		map[string][]string{
			"include": {"api.example.org", "example.org"},
			"exclude": {"api.example.org"},
		},
	},
	{
		[]string{"eXaMPle.ORG", "API.example.ORG"},
		[]string{"api.example.org"},
		[]string{"foobar.API.Example.Org"},
		false,
		map[string][]string{
			"include": {"api.example.org", "example.org"},
			"exclude": {"api.example.org"},
		},
	},
}

var regexDomainFilterTests = []regexDomainFilterTest{
	{
		regexp.MustCompile(`\.org$`),
		regexp.MustCompile(""),
		[]string{"foo.org", "bar.org", "foo.bar.org"},
		true,
		map[string]string{
			"regexInclude": "\\.org$",
		},
	},
	{
		regexp.MustCompile(`\.bar\.org$`),
		regexp.MustCompile(""),
		[]string{"foo.org", "bar.org", "example.com"},
		false,
		map[string]string{
			"regexInclude": "\\.bar\\.org$",
		},
	},
	{
		regexp.MustCompile(`(?:foo|bar)\.org$`),
		regexp.MustCompile(""),
		[]string{"foo.org", "bar.org", "example.foo.org", "example.bar.org", "a.example.foo.org", "a.example.bar.org"},
		true,
		map[string]string{
			"regexInclude": "(?:foo|bar)\\.org$",
		},
	},
	{
		regexp.MustCompile("(?:😍|🤩)\\.org$"),
		regexp.MustCompile(""),
		[]string{"😍.org", "xn--r28h.org", "🤩.org", "example.😍.org", "example.🤩.org", "a.example.xn--r28h.org", "a.example.🤩.org"},
		true,
		map[string]string{
			"regexInclude": "(?:😍|🤩)\\.org$",
		},
	},
	{
		regexp.MustCompile("(?:😍|🤩)\\.org$"),
		regexp.MustCompile("^example\\.(?:😍|🤩)\\.org$"),
		[]string{"example.😍.org", "example.🤩.org"},
		false,
		map[string]string{
			"regexInclude": "(?:😍|🤩)\\.org$",
			"regexExclude": "^example\\.(?:😍|🤩)\\.org$",
		},
	},
	{
		regexp.MustCompile("(?:foo|bar)\\.org$"),
		regexp.MustCompile("^example\\.(?:foo|bar)\\.org$"),
		[]string{"foo.org", "bar.org", "a.example.foo.org", "a.example.bar.org"},
		true,
		map[string]string{
			"regexInclude": `(?:foo|bar)\.org$`,
			"regexExclude": `^example\.(?:foo|bar)\.org$`,
		},
	},
	{
		regexp.MustCompile(`(?:foo|bar)\.org$`),
		regexp.MustCompile(`^example\.(?:foo|bar)\.org$`),
		[]string{"example.foo.org", "example.bar.org"},
		false,
		map[string]string{
			"regexInclude": "(?:foo|bar)\\.org$",
			"regexExclude": "^example\\.(?:foo|bar)\\.org$",
		},
	},
	{
		regexp.MustCompile(`(?:foo|bar)\.org$`),
		regexp.MustCompile(`^example\.(?:foo|bar)\.org$`),
		[]string{"foo.org", "bar.org", "a.example.foo.org", "a.example.bar.org"},
		true,
		map[string]string{
			"regexInclude": "(?:foo|bar)\\.org$",
			"regexExclude": "^example\\.(?:foo|bar)\\.org$",
		},
	},
}

func TestDomainFilterMatch(t *testing.T) {
	for i, tt := range domainFilterTests {
		if len(tt.exclusions) > 0 {
			t.Logf("NewDomainFilter() doesn't support exclusions - skipping test %+v", tt)
			continue
		}
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			domainFilter := NewDomainFilter(tt.domainFilter)

			assertSerializes(t, domainFilter, tt.expectedSerialization)
			deserialized := deserialize(t, map[string][]string{
				"include": tt.domainFilter,
			})

			for _, domain := range tt.domains {
				assert.Equal(t, tt.expected, domainFilter.Match(domain), "%v", domain)
				assert.Equal(t, tt.expected, domainFilter.Match(domain+"."), "%v", domain+".")

				assert.Equal(t, tt.expected, deserialized.Match(domain), "deserialized %v", domain)
				assert.Equal(t, tt.expected, deserialized.Match(domain+"."), "deserialized %v", domain+".")
			}
		})
	}
}

func TestDomainFilterWithExclusions(t *testing.T) {
	for i, tt := range domainFilterTests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			if len(tt.exclusions) == 0 {
				tt.exclusions = append(tt.exclusions, "")
			}
			domainFilter := NewDomainFilterWithExclusions(tt.domainFilter, tt.exclusions)

			assertSerializes(t, domainFilter, tt.expectedSerialization)
			deserialized := deserialize(t, map[string][]string{
				"include": tt.domainFilter,
				"exclude": tt.exclusions,
			})

			for _, domain := range tt.domains {
				assert.Equal(t, tt.expected, domainFilter.Match(domain), "%v", domain)
				assert.Equal(t, tt.expected, domainFilter.Match(domain+"."), "%v", domain+".")

				assert.Equal(t, tt.expected, deserialized.Match(domain), "deserialized %v", domain)
				assert.Equal(t, tt.expected, deserialized.Match(domain+"."), "deserialized %v", domain+".")
			}
		})
	}
}

func TestDomainFilterMatchWithEmptyFilter(t *testing.T) {
	for _, tt := range domainFilterTests {
		domainFilter := DomainFilter{}
		for i, domain := range tt.domains {
			assert.True(t, domainFilter.Match(domain), "should not fail: %v in test-case #%v", domain, i)
			assert.True(t, domainFilter.Match(domain+"."), "should not fail: %v in test-case #%v", domain+".", i)
		}
	}
}

func TestRegexDomainFilter(t *testing.T) {
	for i, tt := range regexDomainFilterTests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			domainFilter := NewRegexDomainFilter(tt.regex, tt.regexExclusion)

			assertSerializes(t, domainFilter, tt.expectedSerialization)
			deserialized := deserialize(t, map[string]string{
				"regexInclude": tt.regex.String(),
				"regexExclude": tt.regexExclusion.String(),
			})

			for _, domain := range tt.domains {
				assert.Equal(t, tt.expected, domainFilter.Match(domain), "%v", domain)
				assert.Equal(t, tt.expected, domainFilter.Match(domain+"."), "%v", domain+".")

				assert.Equal(t, tt.expected, deserialized.Match(domain), "deserialized %v", domain)
				assert.Equal(t, tt.expected, deserialized.Match(domain+"."), "deserialized %v", domain+".")
			}
		})
	}
}

func TestPrepareFiltersStripsWhitespaceAndDotSuffix(t *testing.T) {
	for _, tt := range []struct {
		input  []string
		output []string
	}{
		{
			[]string{},
			nil,
		},
		{
			[]string{""},
			nil,
		},
		{
			[]string{" ", "   ", ""},
			nil,
		},
		{
			[]string{"  foo   ", "  bar. ", "baz.", "xn--bar-zna"},
			[]string{"foo", "bar", "baz", "øbar"},
		},
		{
			[]string{"foo.bar", "  foo.bar.  ", " foo.bar.baz ", " foo.bar.baz.  "},
			[]string{"foo.bar", "foo.bar", "foo.bar.baz", "foo.bar.baz"},
		},
	} {
		t.Run("test string", func(t *testing.T) {
			assert.Equal(t, tt.output, prepareFilters(tt.input))
		})
	}
}

func TestMatchFilterReturnsProperEmptyVal(t *testing.T) {
	emptyFilters := []string{}
	assert.True(t, matchFilter(emptyFilters, "somedomain.com", true))
	assert.False(t, matchFilter(emptyFilters, "somedomain.com", false))
}

func TestDomainFilterIsConfigured(t *testing.T) {
	for i, tt := range []struct {
		filters  []string
		exclude  []string
		expected bool
	}{
		{
			[]string{""},
			[]string{""},
			false,
		},
		{
			[]string{"    "},
			[]string{"    "},
			false,
		},
		{
			[]string{"", ""},
			[]string{""},
			false,
		},
		{
			[]string{" . "},
			[]string{" . "},
			false,
		},
		{
			[]string{" notempty.com "},
			[]string{"  "},
			true,
		},
		{
			[]string{" notempty.com "},
			[]string{"  thisdoesntmatter.com "},
			true,
		},
		{
			[]string{""},
			[]string{"  thisdoesntmatter.com "},
			true,
		},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			df := NewDomainFilterWithExclusions(tt.filters, tt.exclude)
			assert.Equal(t, tt.expected, df.IsConfigured())
		})
	}
}

func TestRegexDomainFilterIsConfigured(t *testing.T) {
	for i, tt := range []struct {
		regex        string
		regexExclude string
		expected     bool
	}{
		{
			"",
			"",
			false,
		},
		{
			"(?:foo|bar)\\.org$",
			"",
			true,
		},
		{
			"",
			"\\.org$",
			true,
		},
		{
			"(?:foo|bar)\\.org$",
			"\\.org$",
			true,
		},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			df := NewRegexDomainFilter(regexp.MustCompile(tt.regex), regexp.MustCompile(tt.regexExclude))
			assert.Equal(t, tt.expected, df.IsConfigured())
		})
	}
}

func TestDomainFilterDeserializeError(t *testing.T) {
	for _, tt := range []struct {
		name          string
		serialized    map[string]interface{}
		expectedError string
	}{
		{
			name: "invalid json",
			serialized: map[string]interface{}{
				"include": 3,
			},
			expectedError: "json: cannot unmarshal number into Go struct field domainFilterSerde.include of type []string",
		},
		{
			name: "include and regex",
			serialized: map[string]interface{}{
				"include":      []string{"example.com"},
				"regexInclude": "example.com",
			},
			expectedError: "cannot have both domain list and regex",
		},
		{
			name: "exclude and regex",
			serialized: map[string]interface{}{
				"exclude":      []string{"example.com"},
				"regexInclude": "example.com",
			},
			expectedError: "cannot have both domain list and regex",
		},
		{
			name: "include and regexExclude",
			serialized: map[string]interface{}{
				"include":      []string{"example.com"},
				"regexExclude": "example.com",
			},
			expectedError: "cannot have both domain list and regex",
		},
		{
			name: "exclude and regexExclude",
			serialized: map[string]interface{}{
				"exclude":      []string{"example.com"},
				"regexExclude": "example.com",
			},
			expectedError: "cannot have both domain list and regex",
		},
		{
			name: "invalid regex",
			serialized: map[string]interface{}{
				"regexInclude": "*",
			},
			expectedError: "invalid regexInclude: error parsing regexp: missing argument to repetition operator: `*`",
		},
		{
			name: "invalid regexExclude",
			serialized: map[string]interface{}{
				"regexExclude": "*",
			},
			expectedError: "invalid regexExclude: error parsing regexp: missing argument to repetition operator: `*`",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var deserialized DomainFilter
			toJson, _ := json.Marshal(tt.serialized)
			err := json.Unmarshal(toJson, &deserialized)
			assert.EqualError(t, err, tt.expectedError)
		})
	}
}

func assertSerializes[T any](t *testing.T, domainFilter *DomainFilter, expectedSerialization map[string]T) {
	serialized, err := json.Marshal(domainFilter)
	assert.NoError(t, err, "serializing")
	expected, err := json.Marshal(expectedSerialization)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(serialized), "json serialization")
}

func deserialize[T any](t *testing.T, serialized map[string]T) *DomainFilter {
	inJson, err := json.Marshal(serialized)
	require.NoError(t, err)
	var deserialized DomainFilter
	err = json.Unmarshal(inJson, &deserialized)
	assert.NoError(t, err, "deserializing")

	return &deserialized
}

func TestDomainFilterMatchParent(t *testing.T) {
	parentMatchTests := []domainFilterTest{
		{
			[]string{"a.example.com."},
			[]string{},
			[]string{"example.com"},
			true,
			map[string][]string{
				"include": {"a.example.com"},
			},
		},
		{
			[]string{" a.example.com "},
			[]string{},
			[]string{"example.com"},
			true,
			map[string][]string{
				"include": {"a.example.com"},
			},
		},
		{
			[]string{""},
			[]string{},
			[]string{"example.com"},
			true,
			map[string][]string{},
		},
		{
			[]string{".a.example.com."},
			[]string{},
			[]string{"example.com"},
			false,
			map[string][]string{
				"include": {".a.example.com"},
			},
		},
		{
			[]string{"a.example.com.", "b.example.com"},
			[]string{},
			[]string{"example.com"},
			true,
			map[string][]string{
				"include": {"a.example.com", "b.example.com"},
			},
		},
		{
			[]string{"a.xn--c1yn36f.æøå.", "b.點看.xn--5cab8c", "c.點看.æøå"},
			[]string{},
			[]string{"xn--c1yn36f.xn--5cab8c"},
			true,
			map[string][]string{
				"include": {"a.點看.æøå", "b.點看.æøå", "c.點看.æøå"},
			},
		},
		{
			[]string{"punycode.xn--c1yn36f.local", "å.點看.local.", "ø.點看.local"},
			[]string{},
			[]string{"點看.local"},
			true,
			map[string][]string{
				"include": {"punycode.點看.local", "å.點看.local", "ø.點看.local"},
			},
		},
		{
			[]string{"a.example.com"},
			[]string{},
			[]string{"b.example.com"},
			false,
			map[string][]string{
				"include": {"a.example.com"},
			},
		},
		{
			[]string{"example.com"},
			[]string{},
			[]string{"example.com"},
			false,
			map[string][]string{
				"include": {"example.com"},
			},
		},
		{
			[]string{"example.com"},
			[]string{},
			[]string{"anexample.com"},
			false,
			map[string][]string{
				"include": {"example.com"},
			},
		},
		{
			[]string{""},
			[]string{},
			[]string{""},
			true,
			map[string][]string{},
		},
	}
	for i, tt := range parentMatchTests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			domainFilter := NewDomainFilterWithExclusions(tt.domainFilter, tt.exclusions)

			assertSerializes(t, domainFilter, tt.expectedSerialization)
			deserialized := deserialize(t, map[string][]string{
				"include": tt.domainFilter,
				"exclude": tt.exclusions,
			})

			for _, domain := range tt.domains {
				assert.Equal(t, tt.expected, domainFilter.MatchParent(domain), "%v", domain)
				assert.Equal(t, tt.expected, domainFilter.MatchParent(domain+"."), "%v", domain+".")

				assert.Equal(t, tt.expected, deserialized.MatchParent(domain), "deserialized %v", domain)
				assert.Equal(t, tt.expected, deserialized.MatchParent(domain+"."), "deserialized %v", domain+".")
			}
		})
	}
}

func TestSimpleDomainFilterWithExclusion(t *testing.T) {
	test := []struct {
		domainFilter    []string
		exclusionFilter []string
		domains         []string
		want            []string
	}{
		{
			domainFilter:    []string{"ex.com"},
			exclusionFilter: []string{"subdomain.ex.com"},
			domains:         []string{"subdomain.ex.com", "ex.com", "subdomain.ex.com.", ".subdomain.ex.com", "one.subdomain.ex.com", "ex.com."},
			want:            []string{"ex.com", "ex.com."},
		},
		{
			domainFilter:    []string{"ex.com"},
			exclusionFilter: []string{},
			domains:         []string{"subdomain.ex.com", "ex.com", "subdomain.ex.com.", ".subdomain.ex.com", "one.subdomain.ex.com", "ex.com."},
			want:            []string{"subdomain.ex.com", "ex.com", "subdomain.ex.com.", ".subdomain.ex.com", "one.subdomain.ex.com", "ex.com."},
		},
		{
			domainFilter:    []string{"ex.com"},
			exclusionFilter: []string{"one.subdomain.ex.com"},
			domains:         []string{"subdomain.ex.com", "ex.com", "subdomain.ex.com.", ".subdomain.ex.com", "one.subdomain.ex.com", "ex.com."},
			want:            []string{"subdomain.ex.com", "ex.com", "subdomain.ex.com.", ".subdomain.ex.com", "ex.com."},
		},
		{
			domainFilter:    []string{"ex.com"},
			exclusionFilter: []string{".ex.com"},
			domains:         []string{"subdomain.ex.com", "ex.com", "subdomain.ex.com.", ".subdomain.ex.com", "one.subdomain.ex.com", "ex.com."},
			want:            []string{"ex.com", "ex.com."},
		},
	}

	for _, tt := range test {
		t.Run(fmt.Sprintf("include:%s-exclude:%s", strings.Join(tt.domainFilter, "_"), strings.Join(tt.exclusionFilter, "_")), func(t *testing.T) {
			domainFilter := NewDomainFilterWithExclusions(tt.domainFilter, tt.exclusionFilter)
			var got []string
			for _, domain := range tt.domains {
				if domainFilter.Match(domain) {
					got = append(got, domain)
				}
			}
			assert.Len(t, tt.want, len(got))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainFilterNormalizeDomain(t *testing.T) {
	records := []struct {
		dnsName string
		expect  string
	}{
		{
			"3AAAA.FOO.BAR.COM",
			"3aaaa.foo.bar.com",
		},
		{
			"example.foo.com.",
			"example.foo.com",
		},
		{
			"example123.foo.com",
			"example123.foo.com",
		},
		{
			"foo.com.",
			"foo.com",
		},
		{
			"foo123.COM",
			"foo123.com",
		},
		{
			"my-exaMple3.FOO.BAR.COM",
			"my-example3.foo.bar.com",
		},
		{
			"my-example1214.FOO-1235.BAR-foo.COM",
			"my-example1214.foo-1235.bar-foo.com",
		},
		{
			"my-example-my-example-1214.FOO-1235.BAR-foo.COM",
			"my-example-my-example-1214.foo-1235.bar-foo.com",
		},
		{
			"xn--c1yn36f.org.",
			"點看.org",
		},
		{
			"xn--nordic--w1a.xn--xn--kItty-pd34d-hn01b3542b.com",
			"nordic-ø.xn--kitty-點看pd34d.com",
		},
		{
			"xn--nordic--w1a.xn--kItty-pd34d.com",
			"nordic-ø.kitty😸.com",
		},
		{
			"nordic-ø.kitty😸.COM",
			"nordic-ø.kitty😸.com",
		},
		{
			"xn--nordic--w1a.kiTTy😸.com.",
			"nordic-ø.kitty😸.com",
		},
	}
	for _, r := range records {
		gotName := normalizeDomain(r.dnsName)
		assert.Equal(t, r.expect, gotName)
	}
}

func TestMatchTargetFilterReturnsProperEmptyVal(t *testing.T) {
	var emptyFilters []string
	assert.True(t, matchFilter(emptyFilters, "sometarget.com", true))
	assert.False(t, matchFilter(emptyFilters, "sometarget.com", false))
}
