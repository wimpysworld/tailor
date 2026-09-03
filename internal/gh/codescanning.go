package gh

import (
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

// codeScanningSetupResponse holds the managed fields of the code scanning
// default setup response. Runner fields are not managed and are ignored.
type codeScanningSetupResponse struct {
	State       string   `json:"state"`
	QuerySuite  string   `json:"query_suite"`
	ThreatModel string   `json:"threat_model"`
	Languages   []string `json:"languages"`
}

func codeScanningSetupPath(owner, name string) string {
	return fmt.Sprintf("repos/%s/%s/code-scanning/default-setup", owner, name)
}

// ReadCodeScanningSetup reads the code scanning default setup. It returns an
// *ErrSetupSkipped when the feature is not available to the repository.
func ReadCodeScanningSetup(client *api.RESTClient, owner, name string) (*model.CodeScanningSettings, error) {
	var response codeScanningSetupResponse
	if err := readSetup(client, codeScanningSetupPath(owner, name), Op(OpFetchCodeScanningSetup), &response); err != nil {
		return nil, err
	}
	return &model.CodeScanningSettings{
		State:       optionalString(response.State),
		QuerySuite:  optionalString(response.QuerySuite),
		ThreatModel: optionalString(response.ThreatModel),
		Languages:   &response.Languages,
	}, nil
}

// ApplyCodeScanningSetup sends the set fields of desired to the code scanning
// default setup endpoint. It sends nothing when no field is set. It returns an
// *ErrSetupSkipped when the feature is not available or a setup run is in
// progress.
func ApplyCodeScanningSetup(client *api.RESTClient, owner, name string, desired *model.CodeScanningSettings) error {
	body := codeScanningSetupBody(desired)
	if len(body) == 0 {
		return nil
	}
	return patchSetup(client, codeScanningSetupPath(owner, name), Op(OpSetCodeScanningSetup), body)
}

// codeScanningSetupBody builds the PATCH body from the set fields of desired.
// An empty language list is omitted so GitHub detects the languages.
func codeScanningSetupBody(desired *model.CodeScanningSettings) map[string]any {
	body := make(map[string]any)
	if desired.State != nil {
		body["state"] = *desired.State
	}
	if desired.QuerySuite != nil {
		body["query_suite"] = *desired.QuerySuite
	}
	if desired.ThreatModel != nil {
		body["threat_model"] = *desired.ThreatModel
	}
	if languages := sortedLanguages(desired.Languages); languages != nil {
		body["languages"] = languages
	}
	return body
}
