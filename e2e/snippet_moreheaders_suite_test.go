package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	moreHeadersIngressName = "snippet-more-headers-test"
	moreHeadersTraefikHost = moreHeadersIngressName + ".traefik.local"
	moreHeadersNginxHost   = moreHeadersIngressName + ".nginx.local"
)

type SnippetMoreHeadersSuite struct {
	BaseSuite
}

func TestSnippetMoreHeadersSuite(t *testing.T) {
	suite.Run(t, new(SnippetMoreHeadersSuite))
}

func (s *SnippetMoreHeadersSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/server-snippet": `
more_set_headers "X-Colon-Clear: colon-value";
more_set_headers "X-NoColon-Clear: nocolon-value";
more_set_headers "X-Server-Cross: server-cross-value";
`,
		"nginx.ingress.kubernetes.io/configuration-snippet": `
more_set_headers "X-Multi-A: a-val" "X-Multi-B: b-val";
more_set_input_headers "X-Input-Multi-A: ia-val" "X-Input-Multi-B: ib-val";
more_set_input_headers "X-Clear-Input";
more_set_headers "X-To-Clear: clear-me";
more_clear_headers "X-To-Clear";
more_set_headers "X-Clear-One: one-val";
more_set_headers "X-Clear-Two: two-val";
more_set_headers "X-Keep-This: keep-val";
more_clear_headers "X-Clear-One" "X-Clear-Two";
more_set_headers "X-Wild-One: w1";
more_set_headers "X-Wild-Two: w2";
more_set_headers "X-Other-Resp: other";
more_clear_headers "X-Wild-*";
more_clear_input_headers "X-Secret";
more_clear_input_headers "X-Prefix-*";
more_set_headers "X-Colon-Clear:";
more_set_headers "X-NoColon-Clear";
more_clear_headers "X-Server-Cross";
`,
	}

	err := s.traefik.DeployIngress(moreHeadersIngressName, moreHeadersTraefikHost, annotations)
	require.NoError(s.T(), err, "deploy more-headers ingress to traefik cluster")

	err = s.nginx.DeployIngress(moreHeadersIngressName, moreHeadersNginxHost, annotations)
	require.NoError(s.T(), err, "deploy more-headers ingress to nginx cluster")

	s.traefik.WaitForIngressReady(s.T(), moreHeadersTraefikHost, 20, 1*time.Second)
	s.nginx.WaitForIngressReady(s.T(), moreHeadersNginxHost, 20, 1*time.Second)
}

func (s *SnippetMoreHeadersSuite) TearDownSuite() {
	_ = s.traefik.DeleteIngress(moreHeadersIngressName)
	_ = s.nginx.DeleteIngress(moreHeadersIngressName)
}

func (s *SnippetMoreHeadersSuite) request(headers map[string]string) (traefikResp, nginxResp *Response) {
	s.T().Helper()

	traefikResp = s.traefik.MakeRequest(s.T(), moreHeadersTraefikHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), traefikResp, "traefik response should not be nil")

	nginxResp = s.nginx.MakeRequest(s.T(), moreHeadersNginxHost, http.MethodGet, "/", headers, 3, 1*time.Second)
	require.NotNil(s.T(), nginxResp, "nginx response should not be nil")

	return traefikResp, nginxResp
}

// --- more_set_headers tests ---

func (s *SnippetMoreHeadersSuite) TestMoreSetHeadersMultiple() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Multi-A", "X-Multi-B"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
	}
	assert.Equal(s.T(), "a-val", traefikResp.ResponseHeaders.Get("X-Multi-A"))
	assert.Equal(s.T(), "b-val", traefikResp.ResponseHeaders.Get("X-Multi-B"))
}

func (s *SnippetMoreHeadersSuite) TestMoreSetHeadersClearingWithColon() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Colon-Clear"),
		traefikResp.ResponseHeaders.Get("X-Colon-Clear"),
		"X-Colon-Clear mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Colon-Clear"),
		"X-Colon-Clear should be cleared by config-snippet")
}

func (s *SnippetMoreHeadersSuite) TestMoreSetHeadersClearingWithoutColon() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-NoColon-Clear"),
		traefikResp.ResponseHeaders.Get("X-NoColon-Clear"),
		"X-NoColon-Clear mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-NoColon-Clear"),
		"X-NoColon-Clear should be cleared by config-snippet")
}

// --- more_set_input_headers tests ---

func (s *SnippetMoreHeadersSuite) TestMoreSetInputHeadersMultiple() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Input-Multi-A", "X-Input-Multi-B"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
	}
	assert.Equal(s.T(), "ia-val", traefikResp.RequestHeaders["X-Input-Multi-A"])
	assert.Equal(s.T(), "ib-val", traefikResp.RequestHeaders["X-Input-Multi-B"])
}

func (s *SnippetMoreHeadersSuite) TestMoreSetInputHeadersClearing() {
	// Send X-Clear-Input with a value; the config-snippet should clear it.
	traefikResp, nginxResp := s.request(map[string]string{
		"X-Clear-Input": "original-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Clear-Input"],
		traefikResp.RequestHeaders["X-Clear-Input"],
		"X-Clear-Input mismatch",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Clear-Input"],
		"X-Clear-Input should be cleared")
}

// --- more_clear_headers tests ---

func (s *SnippetMoreHeadersSuite) TestMoreClearHeadersSingle() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-To-Clear"),
		traefikResp.ResponseHeaders.Get("X-To-Clear"),
		"X-To-Clear mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-To-Clear"),
		"X-To-Clear should be cleared")
}

func (s *SnippetMoreHeadersSuite) TestMoreClearHeadersMultiple() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Clear-One", "X-Clear-Two"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
		assert.Empty(s.T(), traefikResp.ResponseHeaders.Get(header),
			"%s should be cleared", header)
	}

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Keep-This"),
		traefikResp.ResponseHeaders.Get("X-Keep-This"),
		"X-Keep-This mismatch",
	)
	assert.Equal(s.T(), "keep-val", traefikResp.ResponseHeaders.Get("X-Keep-This"))
}

func (s *SnippetMoreHeadersSuite) TestMoreClearHeadersWildcard() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Wild-One", "X-Wild-Two"} {
		assert.Equal(s.T(),
			nginxResp.ResponseHeaders.Get(header),
			traefikResp.ResponseHeaders.Get(header),
			"response header %s mismatch", header,
		)
		assert.Empty(s.T(), traefikResp.ResponseHeaders.Get(header),
			"%s should be cleared by wildcard", header)
	}

	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Other-Resp"),
		traefikResp.ResponseHeaders.Get("X-Other-Resp"),
		"X-Other-Resp mismatch",
	)
	assert.Equal(s.T(), "other", traefikResp.ResponseHeaders.Get("X-Other-Resp"))
}

func (s *SnippetMoreHeadersSuite) TestMoreClearHeadersCrossSnippet() {
	traefikResp, nginxResp := s.request(nil)

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.ResponseHeaders.Get("X-Server-Cross"),
		traefikResp.ResponseHeaders.Get("X-Server-Cross"),
		"X-Server-Cross mismatch",
	)
	assert.Empty(s.T(), traefikResp.ResponseHeaders.Get("X-Server-Cross"),
		"X-Server-Cross should be cleared by config-snippet")
}

// --- more_clear_input_headers tests ---

func (s *SnippetMoreHeadersSuite) TestMoreClearInputHeadersSingle() {
	traefikResp, nginxResp := s.request(map[string]string{
		"X-Secret": "secret-value",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")
	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Secret"],
		traefikResp.RequestHeaders["X-Secret"],
		"X-Secret mismatch",
	)
	assert.Empty(s.T(), traefikResp.RequestHeaders["X-Secret"],
		"X-Secret should be cleared from request")
}

func (s *SnippetMoreHeadersSuite) TestMoreClearInputHeadersWildcard() {
	traefikResp, nginxResp := s.request(map[string]string{
		"X-Prefix-One": "val1",
		"X-Prefix-Two": "val2",
		"X-Other":      "other",
	})

	assert.Equal(s.T(), nginxResp.StatusCode, traefikResp.StatusCode, "status code mismatch")

	for _, header := range []string{"X-Prefix-One", "X-Prefix-Two"} {
		assert.Equal(s.T(),
			nginxResp.RequestHeaders[header],
			traefikResp.RequestHeaders[header],
			"request header %s mismatch", header,
		)
		assert.Empty(s.T(), traefikResp.RequestHeaders[header],
			"%s should be cleared by wildcard", header)
	}

	assert.Equal(s.T(),
		nginxResp.RequestHeaders["X-Other"],
		traefikResp.RequestHeaders["X-Other"],
		"X-Other mismatch",
	)
	assert.Equal(s.T(), "other", traefikResp.RequestHeaders["X-Other"])
}
