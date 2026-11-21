package xray

import (
	"strings"
	"testing"
)

// TestValidateCapturesStdout: xray writes config errors to STDOUT, so
// Validate must surface stdout (CombinedOutput), not stderr alone.
func TestValidateCapturesStdout(t *testing.T) {
	bin := fakeXray(t, `echo 'VLESS settings: please add/set "decryption":"none" to every settings'
exit 23
`)
	err := Validate(bin, []byte("{}"))
	if err == nil {
		t.Fatal("Validate(fake xray exit 23) = nil error, want error")
	}
	if !strings.Contains(err.Error(), `"decryption":"none" to every settings`) {
		t.Errorf("Validate error missing xray stdout text: %v", err)
	}
}
