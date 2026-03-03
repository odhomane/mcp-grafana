package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSilenceDuration_Days(t *testing.T) {
	d, err := parseSilenceDuration("30d")
	require.NoError(t, err)
	require.Equal(t, "720h0m0s", d.String())
}

func TestCreateSilenceParamsValidate_AllowsDefaultCreatorAndComment(t *testing.T) {
	err := (CreateSilenceParams{
		Matchers: []SilenceMatcherInput{{Name: "customer_id", Value: "G002"}},
		Duration: ptr("30d"),
	}).validate()

	require.NoError(t, err)
}

func TestCreateSilenceParamsValidate_RequiresEndOrDuration(t *testing.T) {
	err := (CreateSilenceParams{
		Matchers: []SilenceMatcherInput{{Name: "customer_id", Value: "G002"}},
	}).validate()

	require.EqualError(t, err, "either endsAt or duration is required")
}

func ptr(s string) *string {
	return &s
}
