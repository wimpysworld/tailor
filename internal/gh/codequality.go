package gh

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

// codeQualitySetupResponse holds the managed fields of the Code Quality setup
// response. Runner and AI findings fields are not managed and are ignored.
type codeQualitySetupResponse struct {
	State     string   `json:"state"`
	Languages []string `json:"languages"`
}

func codeQualitySetupPath(owner, name string) string {
	return fmt.Sprintf("repos/%s/%s/code-quality/setup", owner, name)
}

// ReadCodeQualitySetup reads the Code Quality setup. It returns an
// *ErrSetupSkipped when the feature is not available to the repository.
func ReadCodeQualitySetup(client *api.RESTClient, owner, name string) (*model.CodeQualitySettings, error) {
	var response codeQualitySetupResponse
	if err := readSetup(client, codeQualitySetupPath(owner, name), Op(OpFetchCodeQualitySetup), &response); err != nil {
		return nil, err
	}
	return &model.CodeQualitySettings{
		State:     optionalString(response.State),
		Languages: &response.Languages,
	}, nil
}

// ApplyCodeQualitySetup sends declared fields to the Code Quality endpoint,
// excluding empty language lists. An empty body sends no request.
// It returns *ErrSetupSkipped when the feature is unavailable or setup is in progress.
func ApplyCodeQualitySetup(client *api.RESTClient, owner, name string, desired *model.CodeQualitySettings) error {
	body := codeQualitySetupBody(desired)
	if len(body) == 0 {
		return nil
	}
	return patchSetup(client, codeQualitySetupPath(owner, name), Op(OpSetCodeQualitySetup), body)
}

// codeQualitySetupBody builds the PATCH body from the set fields of desired.
// An empty language list is omitted so GitHub detects the languages.
func codeQualitySetupBody(desired *model.CodeQualitySettings) map[string]any {
	body := make(map[string]any)
	if desired.State != nil {
		body["state"] = *desired.State
	}
	if languages := sortedLanguages(desired.Languages); languages != nil {
		body["languages"] = languages
	}
	return body
}
