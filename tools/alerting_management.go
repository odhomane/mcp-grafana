package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/grafana-openapi-client-go/client/provisioning"
	"github.com/grafana/grafana-openapi-client-go/models"
	"github.com/mark3labs/mcp-go/mcp"

	mcpgrafana "github.com/grafana/mcp-grafana"
)

type SilenceMatcherInput struct {
	Name    string `json:"name" jsonschema:"required,description=Label name to match"`
	Value   string `json:"value" jsonschema:"required,description=Label value to match"`
	IsRegex bool   `json:"isRegex,omitempty" jsonschema:"description=Whether value should be treated as a regex"`
	IsEqual *bool  `json:"isEqual,omitempty" jsonschema:"description=Set false for negative match (!= or !~ depending on isRegex). Defaults to true"`
}

type ListSilencesParams struct {
	DatasourceUID *string  `json:"datasourceUid,omitempty" jsonschema:"description=Optional Alertmanager datasource UID. If omitted\\, uses Grafana-managed Alertmanager"`
	Filters       []string `json:"filters,omitempty" jsonschema:"description=Optional Alertmanager filter expressions (e.g. ['customer_id=G002'])"`
}

type SilenceSummary struct {
	ID        string                `json:"id"`
	StartsAt  string                `json:"startsAt"`
	EndsAt    string                `json:"endsAt"`
	CreatedBy string                `json:"createdBy"`
	Comment   string                `json:"comment"`
	State     string                `json:"state,omitempty"`
	Matchers  []SilenceMatcherInput `json:"matchers"`
}

func listSilences(ctx context.Context, args ListSilencesParams) ([]SilenceSummary, error) {
	client, err := newAlertingClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list silences: %w", err)
	}

	silences, err := client.ListSilences(ctx, args.DatasourceUID, args.Filters)
	if err != nil {
		return nil, fmt.Errorf("list silences: %w", err)
	}

	result := make([]SilenceSummary, 0, len(silences))
	for _, silence := range silences {
		var matchers []SilenceMatcherInput
		for _, m := range silence.Matchers {
			isEqual := m.IsEqual
			matchers = append(matchers, SilenceMatcherInput{
				Name:    m.Name,
				Value:   m.Value,
				IsRegex: m.IsRegex,
				IsEqual: &isEqual,
			})
		}

		state := ""
		if silence.Status != nil {
			state = silence.Status.State
		}

		result = append(result, SilenceSummary{
			ID:        silence.ID,
			StartsAt:  silence.StartsAt,
			EndsAt:    silence.EndsAt,
			CreatedBy: silence.CreatedBy,
			Comment:   silence.Comment,
			State:     state,
			Matchers:  matchers,
		})
	}

	return result, nil
}

var ListSilences = mcpgrafana.MustTool(
	"list_silences",
	"List Alertmanager silences. By default this queries Grafana-managed Alertmanager. You can optionally target an Alertmanager datasource via datasourceUid.",
	listSilences,
	mcp.WithTitleAnnotation("List alerting silences"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

type GetSilenceParams struct {
	SilenceID     string  `json:"silenceId" jsonschema:"required,description=Silence ID"`
	DatasourceUID *string `json:"datasourceUid,omitempty" jsonschema:"description=Optional Alertmanager datasource UID"`
}

func (p GetSilenceParams) validate() error {
	if strings.TrimSpace(p.SilenceID) == "" {
		return fmt.Errorf("silenceId is required")
	}
	return nil
}

func getSilence(ctx context.Context, args GetSilenceParams) (*SilenceSummary, error) {
	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("get silence: %w", err)
	}

	client, err := newAlertingClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get silence: %w", err)
	}

	silence, err := client.GetSilence(ctx, args.DatasourceUID, args.SilenceID)
	if err != nil {
		return nil, fmt.Errorf("get silence: %w", err)
	}

	matchers := make([]SilenceMatcherInput, 0, len(silence.Matchers))
	for _, m := range silence.Matchers {
		isEqual := m.IsEqual
		matchers = append(matchers, SilenceMatcherInput{
			Name:    m.Name,
			Value:   m.Value,
			IsRegex: m.IsRegex,
			IsEqual: &isEqual,
		})
	}

	state := ""
	if silence.Status != nil {
		state = silence.Status.State
	}

	return &SilenceSummary{
		ID:        silence.ID,
		StartsAt:  silence.StartsAt,
		EndsAt:    silence.EndsAt,
		CreatedBy: silence.CreatedBy,
		Comment:   silence.Comment,
		State:     state,
		Matchers:  matchers,
	}, nil
}

var GetSilence = mcpgrafana.MustTool(
	"get_silence",
	"Get a specific Alertmanager silence by ID.",
	getSilence,
	mcp.WithTitleAnnotation("Get silence"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

type CreateSilenceParams struct {
	SilenceID     *string               `json:"silenceId,omitempty" jsonschema:"description=Optional silence ID to update an existing silence"`
	DatasourceUID *string               `json:"datasourceUid,omitempty" jsonschema:"description=Optional Alertmanager datasource UID"`
	Matchers      []SilenceMatcherInput `json:"matchers" jsonschema:"required,description=Alert label matchers for this silence"`
	StartsAt      *string               `json:"startsAt,omitempty" jsonschema:"description=RFC3339 start time. Defaults to now"`
	EndsAt        *string               `json:"endsAt,omitempty" jsonschema:"description=RFC3339 end time. Required unless duration is provided"`
	Duration      *string               `json:"duration,omitempty" jsonschema:"description=Duration from start time (e.g. '240h' or '10d'). Required if endsAt is not set"`
	CreatedBy     string                `json:"createdBy,omitempty" jsonschema:"description=Creator identifier or username. Defaults to 'mcp-grafana'"`
	Comment       string                `json:"comment,omitempty" jsonschema:"description=Human-readable reason for the silence. Defaults to auto-generated text"`
}

func parseSilenceDuration(raw string) (time.Duration, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if strings.HasSuffix(v, "d") {
		days := strings.TrimSuffix(v, "d")
		var dayCount int
		_, err := fmt.Sscanf(days, "%d", &dayCount)
		if err != nil || dayCount <= 0 {
			return 0, fmt.Errorf("invalid day duration %q", raw)
		}
		return time.Duration(dayCount) * 24 * time.Hour, nil
	}
	return time.ParseDuration(v)
}

func (p CreateSilenceParams) validate() error {
	if len(p.Matchers) == 0 {
		return fmt.Errorf("at least one matcher is required")
	}
	if p.EndsAt == nil && p.Duration == nil {
		return fmt.Errorf("either endsAt or duration is required")
	}
	return nil
}

func createSilence(ctx context.Context, args CreateSilenceParams) (map[string]string, error) {
	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("create silence: %w", err)
	}

	startAt := time.Now().UTC()
	if args.StartsAt != nil && strings.TrimSpace(*args.StartsAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*args.StartsAt))
		if err != nil {
			return nil, fmt.Errorf("create silence: invalid startsAt: %w", err)
		}
		startAt = parsed.UTC()
	}

	var endAt time.Time
	if args.EndsAt != nil && strings.TrimSpace(*args.EndsAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*args.EndsAt))
		if err != nil {
			return nil, fmt.Errorf("create silence: invalid endsAt: %w", err)
		}
		endAt = parsed.UTC()
	} else {
		dur, err := parseSilenceDuration(*args.Duration)
		if err != nil {
			return nil, fmt.Errorf("create silence: invalid duration: %w", err)
		}
		endAt = startAt.Add(dur)
	}

	if !endAt.After(startAt) {
		return nil, fmt.Errorf("create silence: end time must be after start time")
	}

	matchers := make([]alertmanagerMatcher, 0, len(args.Matchers))
	matcherSummary := make([]string, 0, len(args.Matchers))
	for _, m := range args.Matchers {
		if strings.TrimSpace(m.Name) == "" {
			return nil, fmt.Errorf("create silence: matcher name is required")
		}
		if strings.TrimSpace(m.Value) == "" {
			return nil, fmt.Errorf("create silence: matcher value is required")
		}
		isEqual := true
		if m.IsEqual != nil {
			isEqual = *m.IsEqual
		}
		matchers = append(matchers, alertmanagerMatcher{
			Name:    m.Name,
			Value:   m.Value,
			IsRegex: m.IsRegex,
			IsEqual: isEqual,
		})

		op := "="
		if !isEqual {
			op = "!="
		}
		if m.IsRegex {
			op = "=~"
			if !isEqual {
				op = "!~"
			}
		}
		matcherSummary = append(matcherSummary, fmt.Sprintf("%s%s%s", m.Name, op, m.Value))
	}

	createdBy := strings.TrimSpace(args.CreatedBy)
	if createdBy == "" {
		createdBy = "mcp-grafana"
	}

	comment := strings.TrimSpace(args.Comment)
	if comment == "" {
		comment = fmt.Sprintf("Silence via MCP for %s", strings.Join(matcherSummary, ", "))
	}

	payload := alertmanagerPostableSilence{
		Matchers:  matchers,
		StartsAt:  startAt.Format(time.RFC3339),
		EndsAt:    endAt.Format(time.RFC3339),
		CreatedBy: createdBy,
		Comment:   comment,
	}
	if args.SilenceID != nil && strings.TrimSpace(*args.SilenceID) != "" {
		payload.ID = strings.TrimSpace(*args.SilenceID)
	}

	client, err := newAlertingClientFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("create silence: %w", err)
	}

	silenceID, err := client.CreateSilence(ctx, args.DatasourceUID, payload)
	if err != nil {
		return nil, fmt.Errorf("create silence: %w", err)
	}

	return map[string]string{
		"silenceId": silenceID,
		"startsAt":  payload.StartsAt,
		"endsAt":    payload.EndsAt,
	}, nil
}

var CreateSilence = mcpgrafana.MustTool(
	"create_silence",
	"Create or update an Alertmanager silence. Supports duration format like '240h' or '10d'.",
	createSilence,
	mcp.WithTitleAnnotation("Create silence"),
	mcp.WithDestructiveHintAnnotation(true),
)

type DeleteSilenceParams struct {
	SilenceID     string  `json:"silenceId" jsonschema:"required,description=Silence ID to expire/delete"`
	DatasourceUID *string `json:"datasourceUid,omitempty" jsonschema:"description=Optional Alertmanager datasource UID"`
}

func (p DeleteSilenceParams) validate() error {
	if strings.TrimSpace(p.SilenceID) == "" {
		return fmt.Errorf("silenceId is required")
	}
	return nil
}

func deleteSilence(ctx context.Context, args DeleteSilenceParams) (string, error) {
	if err := args.validate(); err != nil {
		return "", fmt.Errorf("delete silence: %w", err)
	}

	client, err := newAlertingClientFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("delete silence: %w", err)
	}

	if err := client.DeleteSilence(ctx, args.DatasourceUID, args.SilenceID); err != nil {
		return "", fmt.Errorf("delete silence: %w", err)
	}

	return fmt.Sprintf("Silence %s deleted successfully", args.SilenceID), nil
}

var DeleteSilence = mcpgrafana.MustTool(
	"delete_silence",
	"Delete (expire) an Alertmanager silence by ID.",
	deleteSilence,
	mcp.WithTitleAnnotation("Delete silence"),
	mcp.WithDestructiveHintAnnotation(true),
)

type GetNotificationPolicyParams struct{}

func getNotificationPolicy(ctx context.Context, _ GetNotificationPolicyParams) (*models.Route, error) {
	c := mcpgrafana.GrafanaClientFromContext(ctx)
	res, err := c.Provisioning.GetPolicyTree()
	if err != nil {
		return nil, fmt.Errorf("get notification policy: %w", err)
	}
	return res.Payload, nil
}

var GetNotificationPolicy = mcpgrafana.MustTool(
	"get_notification_policy",
	"Get the Grafana notification policy tree (routing configuration).",
	getNotificationPolicy,
	mcp.WithTitleAnnotation("Get notification policy"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

type UpdateNotificationPolicyParams struct {
	Policy            map[string]any `json:"policy" jsonschema:"required,description=Full notification policy route tree JSON object"`
	DisableProvenance *bool          `json:"disableProvenance,omitempty" jsonschema:"description=If true\\, keep policy editable in UI by setting X-Disable-Provenance header. Defaults to true"`
}

func updateNotificationPolicy(ctx context.Context, args UpdateNotificationPolicyParams) (string, error) {
	if args.Policy == nil {
		return "", fmt.Errorf("update notification policy: policy is required")
	}

	rawPolicy, err := json.Marshal(args.Policy)
	if err != nil {
		return "", fmt.Errorf("update notification policy: invalid policy JSON: %w", err)
	}
	var route models.Route
	if err := json.Unmarshal(rawPolicy, &route); err != nil {
		return "", fmt.Errorf("update notification policy: unable to parse policy tree: %w", err)
	}

	c := mcpgrafana.GrafanaClientFromContext(ctx)
	params := provisioning.NewPutPolicyTreeParams().WithContext(ctx).WithBody(&route)

	disableProvenance := true
	if args.DisableProvenance != nil {
		disableProvenance = *args.DisableProvenance
	}
	if disableProvenance {
		header := "true"
		params = params.WithXDisableProvenance(&header)
	}

	if _, err := c.Provisioning.PutPolicyTree(params); err != nil {
		return "", fmt.Errorf("update notification policy: %w", err)
	}

	return "Notification policy updated successfully", nil
}

var UpdateNotificationPolicy = mcpgrafana.MustTool(
	"update_notification_policy",
	"Update the Grafana notification policy tree. This replaces the existing route tree.",
	updateNotificationPolicy,
	mcp.WithTitleAnnotation("Update notification policy"),
	mcp.WithDestructiveHintAnnotation(true),
)

type ResetNotificationPolicyParams struct{}

func resetNotificationPolicy(ctx context.Context, _ ResetNotificationPolicyParams) (string, error) {
	c := mcpgrafana.GrafanaClientFromContext(ctx)
	if _, err := c.Provisioning.ResetPolicyTree(); err != nil {
		return "", fmt.Errorf("reset notification policy: %w", err)
	}
	return "Notification policy reset successfully", nil
}

var ResetNotificationPolicy = mcpgrafana.MustTool(
	"reset_notification_policy",
	"Reset the notification policy tree back to Grafana defaults.",
	resetNotificationPolicy,
	mcp.WithTitleAnnotation("Reset notification policy"),
	mcp.WithDestructiveHintAnnotation(true),
)

type GetContactPointParams struct {
	UID string `json:"uid" jsonschema:"required,description=Contact point UID"`
}

func (p GetContactPointParams) validate() error {
	if strings.TrimSpace(p.UID) == "" {
		return fmt.Errorf("uid is required")
	}
	return nil
}

func getContactPoint(ctx context.Context, args GetContactPointParams) (*models.EmbeddedContactPoint, error) {
	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("get contact point: %w", err)
	}

	c := mcpgrafana.GrafanaClientFromContext(ctx)
	res, err := c.Provisioning.GetContactpoints(provisioning.NewGetContactpointsParams().WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get contact point: %w", err)
	}

	for _, cp := range res.Payload {
		if cp.UID == args.UID {
			return cp, nil
		}
	}

	return nil, fmt.Errorf("get contact point: uid %s not found", args.UID)
}

var GetContactPoint = mcpgrafana.MustTool(
	"get_contact_point",
	"Get a specific Grafana-managed notification contact point by UID.",
	getContactPoint,
	mcp.WithTitleAnnotation("Get contact point"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

type CreateContactPointParams struct {
	UID                   *string        `json:"uid,omitempty" jsonschema:"description=Optional UID for the contact point"`
	Name                  string         `json:"name" jsonschema:"required,description=Contact point name"`
	Type                  string         `json:"type" jsonschema:"required,description=Integration type (e.g. email\\, webhook\\, slack)"`
	Settings              map[string]any `json:"settings" jsonschema:"required,description=Integration-specific settings map"`
	DisableResolveMessage bool           `json:"disableResolveMessage,omitempty" jsonschema:"description=Disable resolved notifications"`
	DisableProvenance     *bool          `json:"disableProvenance,omitempty" jsonschema:"description=If true\\, keep contact point editable in UI. Defaults to true"`
}

func (p CreateContactPointParams) validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(p.Type) == "" {
		return fmt.Errorf("type is required")
	}
	if p.Settings == nil {
		return fmt.Errorf("settings is required")
	}
	return nil
}

func createContactPoint(ctx context.Context, args CreateContactPointParams) (*models.EmbeddedContactPoint, error) {
	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("create contact point: %w", err)
	}

	c := mcpgrafana.GrafanaClientFromContext(ctx)
	cpType := args.Type
	cp := &models.EmbeddedContactPoint{
		Name:                  args.Name,
		Type:                  &cpType,
		Settings:              args.Settings,
		DisableResolveMessage: args.DisableResolveMessage,
	}
	if args.UID != nil && strings.TrimSpace(*args.UID) != "" {
		cp.UID = strings.TrimSpace(*args.UID)
	}

	params := provisioning.NewPostContactpointsParams().WithContext(ctx).WithBody(cp)
	disableProvenance := true
	if args.DisableProvenance != nil {
		disableProvenance = *args.DisableProvenance
	}
	if disableProvenance {
		header := "true"
		params = params.WithXDisableProvenance(&header)
	}

	res, err := c.Provisioning.PostContactpoints(params)
	if err != nil {
		return nil, fmt.Errorf("create contact point: %w", err)
	}
	return res.Payload, nil
}

var CreateContactPoint = mcpgrafana.MustTool(
	"create_contact_point",
	"Create a new Grafana-managed notification contact point.",
	createContactPoint,
	mcp.WithTitleAnnotation("Create contact point"),
	mcp.WithDestructiveHintAnnotation(true),
)

type UpdateContactPointParams struct {
	UID                   string         `json:"uid" jsonschema:"required,description=Existing contact point UID"`
	Name                  string         `json:"name" jsonschema:"required,description=Contact point name"`
	Type                  string         `json:"type" jsonschema:"required,description=Integration type (e.g. email\\, webhook\\, slack)"`
	Settings              map[string]any `json:"settings" jsonschema:"required,description=Integration-specific settings map"`
	DisableResolveMessage bool           `json:"disableResolveMessage,omitempty" jsonschema:"description=Disable resolved notifications"`
	DisableProvenance     *bool          `json:"disableProvenance,omitempty" jsonschema:"description=If true\\, keep contact point editable in UI. Defaults to true"`
}

func (p UpdateContactPointParams) validate() error {
	if strings.TrimSpace(p.UID) == "" {
		return fmt.Errorf("uid is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(p.Type) == "" {
		return fmt.Errorf("type is required")
	}
	if p.Settings == nil {
		return fmt.Errorf("settings is required")
	}
	return nil
}

func updateContactPoint(ctx context.Context, args UpdateContactPointParams) (any, error) {
	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("update contact point: %w", err)
	}

	c := mcpgrafana.GrafanaClientFromContext(ctx)
	cpType := args.Type
	cp := &models.EmbeddedContactPoint{
		UID:                   args.UID,
		Name:                  args.Name,
		Type:                  &cpType,
		Settings:              args.Settings,
		DisableResolveMessage: args.DisableResolveMessage,
	}

	params := provisioning.NewPutContactpointParams().WithContext(ctx).WithUID(args.UID).WithBody(cp)
	disableProvenance := true
	if args.DisableProvenance != nil {
		disableProvenance = *args.DisableProvenance
	}
	if disableProvenance {
		header := "true"
		params = params.WithXDisableProvenance(&header)
	}

	res, err := c.Provisioning.PutContactpoint(params)
	if err != nil {
		return nil, fmt.Errorf("update contact point: %w", err)
	}

	return res.Payload, nil
}

var UpdateContactPoint = mcpgrafana.MustTool(
	"update_contact_point",
	"Update an existing Grafana-managed notification contact point.",
	updateContactPoint,
	mcp.WithTitleAnnotation("Update contact point"),
	mcp.WithDestructiveHintAnnotation(true),
)

type DeleteContactPointParams struct {
	UID string `json:"uid" jsonschema:"required,description=Contact point UID to delete"`
}

func (p DeleteContactPointParams) validate() error {
	if strings.TrimSpace(p.UID) == "" {
		return fmt.Errorf("uid is required")
	}
	return nil
}

func deleteContactPoint(ctx context.Context, args DeleteContactPointParams) (string, error) {
	if err := args.validate(); err != nil {
		return "", fmt.Errorf("delete contact point: %w", err)
	}

	c := mcpgrafana.GrafanaClientFromContext(ctx)
	if _, err := c.Provisioning.DeleteContactpoints(args.UID); err != nil {
		return "", fmt.Errorf("delete contact point: %w", err)
	}

	return fmt.Sprintf("Contact point %s deleted successfully", args.UID), nil
}

var DeleteContactPoint = mcpgrafana.MustTool(
	"delete_contact_point",
	"Delete a Grafana-managed notification contact point by UID.",
	deleteContactPoint,
	mcp.WithTitleAnnotation("Delete contact point"),
	mcp.WithDestructiveHintAnnotation(true),
)
