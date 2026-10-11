package preview

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

const PreviewXRDPath = "platform/crossplane/xrd.yaml"

// The baseline hash pins the trusted XRD shape. A PR may add only one schema
// constraint; the candidate file itself is never applied by the installer.
const baselinePreviewXRDSHA256 = "12f21d86762271374b100df99367b3c91decab1d75b7a6d5b5dfa8acd6e08119"

const reasonCodesBaseline = "reasonCodes:\n                      type: array\n                      items: {type: string}"
const reasonCodesCandidate = "reasonCodes:\n                      type: array\n                      minItems: 1\n                      items: {type: string}"

// ParsePreviewXRD accepts a single bounded validation improvement: a decision
// must carry at least one explanation code. Restoring the baseline must yield
// the exact pinned source, so extra documents or edits fail closed.
func ParsePreviewXRD(content string) (int, error) {
	if strings.Count(content, reasonCodesCandidate) != 1 {
		return 0, errors.New("only decision.reasonCodes minItems: 1 is supported")
	}
	baseline := strings.Replace(content, reasonCodesCandidate, reasonCodesBaseline, 1)
	if digest := fmt.Sprintf("%x", sha256.Sum256([]byte(baseline))); digest != baselinePreviewXRDSHA256 {
		return 0, errors.New("XRD differs from the pinned trusted baseline")
	}
	return 1, nil
}
