package transcdr

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FieldError is one problem with a request field: an output spec refused
// lists every one it found (Error.Errors), and [ValidateOutput] reports the
// same.
type FieldError struct {
	// Param is the field's full path, such as "output.audio.bitrate" or
	// "output.renditions.sizes.0.fit".
	Param   string `json:"param"`
	Message string `json:"message"`
}

// outputRules is the API's table of which fields an output spec needs, as
// listed in GET /v1/capabilities (output.fields, output.groups).
//
//go:embed output_rules.json
var outputRulesJSON []byte

type ruleTerm struct {
	path   string
	values []string
}

type ruleField struct {
	Path     string       `json:"path"`
	Required bool         `json:"required"`
	When     [][][]any    `json:"when"`
	Object   bool         `json:"object"`
	Group    *string      `json:"group"`
	when     [][]ruleTerm // parsed
}

type ruleGroup struct {
	Name    string    `json:"name"`
	Parent  string    `json:"parent"`
	Members []string  `json:"members"`
	When    [][][]any `json:"when"`
	when    [][]ruleTerm
}

var outputRules = func() (r struct {
	Fields []ruleField `json:"fields"`
	Groups []ruleGroup `json:"groups"`
}) {
	if err := json.Unmarshal(outputRulesJSON, &r); err != nil {
		panic("transcdr: output rules: " + err.Error())
	}
	for i := range r.Fields {
		r.Fields[i].when = parseCondition(r.Fields[i].When)
	}
	for i := range r.Groups {
		r.Groups[i].when = parseCondition(r.Groups[i].When)
	}
	return r
}()

func parseCondition(raw [][][]any) [][]ruleTerm {
	out := make([][]ruleTerm, len(raw))
	for i, clause := range raw {
		for _, term := range clause {
			t := ruleTerm{path: term[0].(string)}
			for _, v := range term[1].([]any) {
				t.values = append(t.values, v.(string))
			}
			out[i] = append(out[i], t)
		}
	}
	return out
}

// ValidateOutput checks a whole output spec against the API's table of
// required fields: every field its kind, container, codec and audio handling
// need is present, none is given where it does not apply, and each exclusive
// group has exactly one choice. It reports every problem at once, with the
// params and messages of the API's 422 (error.errors), and nil for a
// complete spec. Values against each other (HDR with 8-bit, MP3 in HLS) and
// plan limits are checked by the API.
func ValidateOutput(spec OutputSpec) []FieldError {
	b, err := json.Marshal(spec)
	if err != nil {
		return []FieldError{{Param: "output", Message: "output could not be encoded: " + err.Error()}}
	}
	return ValidateOutputJSON(b)
}

// ValidateOutputJSON is [ValidateOutput] for a spec given as JSON.
func ValidateOutputJSON(spec json.RawMessage) []FieldError {
	var doc any
	if err := json.Unmarshal(spec, &doc); err != nil {
		return []FieldError{{Param: "output", Message: "output must be an object."}}
	}
	return validateDocument(doc)
}

func fieldError(path, message string) FieldError {
	if path == "" {
		return FieldError{Param: "output", Message: message}
	}
	return FieldError{Param: "output." + path, Message: message}
}

func validateDocument(doc any) []FieldError {
	root, ok := doc.(map[string]any)
	if !ok {
		return []FieldError{fieldError("", "output must be an object.")}
	}
	var errs []FieldError
	switch k := root["kind"].(type) {
	case nil:
		return []FieldError{fieldError("kind", "output.kind is required: video, audio or image.")}
	case string:
		if k != "video" && k != "audio" && k != "image" {
			return []FieldError{fieldError("kind", "output.kind must be video, audio or image.")}
		}
	default:
		return []FieldError{fieldError("kind", "output.kind must be video, audio or image.")}
	}

	privacyMissing := lookup(root, "privacy") == nil
	if privacyMissing {
		errs = append(errs, fieldError("privacy", "output.privacy is required: give privacy.preset (strip_all, strip_location or keep_all), or all of location, capture_time, device and descriptive."))
	}

	var refused []string
	for _, f := range outputRules.Fields {
		if privacyMissing && strings.HasPrefix(f.Path, "privacy") {
			continue
		}
		applies := holds(root, f.when)
		for _, in := range instances(root, f.Path) {
			if isUnder(in.path, refused) {
				continue
			}
			if in.value != nil && !applies {
				refused = append(refused, in.path)
			}
			switch {
			case in.value == nil && applies && f.Required && !f.Object:
				errs = append(errs, fieldError(in.path, fmt.Sprintf("output.%s is required when %s.", in.path, describe(f.when))))
			case in.value != nil && !applies:
				errs = append(errs, fieldError(in.path, fmt.Sprintf("output.%s does not apply here: it applies when %s. Remove it (or set it to null).", in.path, describe(f.when))))
			}
		}
	}

	for _, g := range outputRules.Groups {
		if !holds(root, g.when) {
			continue
		}
		var given []string
		names := make([]string, len(g.Members))
		for i, m := range g.Members {
			if lookup(root, m) != nil {
				given = append(given, m)
			}
			names[i] = m[strings.LastIndex(m, ".")+1:]
		}
		switch {
		case len(given) == 0:
			errs = append(errs, fieldError(g.Parent, fmt.Sprintf("output.%s needs one of %s.", g.Parent, orList(names))))
		case len(given) > 1:
			errs = append(errs, fieldError(given[1], fmt.Sprintf("output.%s takes one of %s, not %s.", g.Parent, orList(names), strings.Join(given, " and "))))
		}
	}
	return errs
}

func isUnder(path string, refused []string) bool {
	for _, r := range refused {
		if strings.HasPrefix(path, r+".") {
			return true
		}
	}
	return false
}

// lookup is the value at a dotted path, nil when absent or null.
func lookup(doc any, path string) any {
	at := doc
	for _, key := range strings.Split(path, ".") {
		m, ok := at.(map[string]any)
		if !ok {
			return nil
		}
		if at, ok = m[key]; !ok {
			return nil
		}
	}
	return at
}

func valueText(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case bool:
		return strconv.FormatBool(x), true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	}
	return "", false
}

func termHolds(doc any, t ruleTerm) bool {
	found := lookup(doc, t.path)
	if len(t.values) == 1 && t.values[0] == "*" {
		return found != nil
	}
	if len(t.values) == 1 && t.values[0] == "!" {
		return found == nil
	}
	matches := func(v any) bool {
		s, ok := valueText(v)
		if !ok {
			return false
		}
		for _, want := range t.values {
			if s == want {
				return true
			}
		}
		return false
	}
	if items, ok := found.([]any); ok {
		for _, item := range items {
			if matches(item) {
				return true
			}
		}
		return false
	}
	return found != nil && matches(found)
}

func holds(doc any, condition [][]ruleTerm) bool {
	for _, clause := range condition {
		all := true
		for _, t := range clause {
			if !termHolds(doc, t) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func describe(condition [][]ruleTerm) string {
	clauses := make([]string, len(condition))
	for i, clause := range condition {
		parts := make([]string, len(clause))
		for j, t := range clause {
			switch {
			case len(t.values) == 1 && t.values[0] == "*":
				parts[j] = t.path + " is given"
			case len(t.values) == 1 && t.values[0] == "!":
				parts[j] = t.path + " is not given"
			default:
				parts[j] = t.path + " is " + orList(t.values)
			}
		}
		clauses[i] = strings.Join(parts, " and ")
	}
	return strings.Join(clauses, ", or ")
}

func orList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

type instance struct {
	path  string
	value any
}

// instances are a field's concrete paths: one for a plain path, one per
// entry below a "[]".
func instances(doc any, path string) []instance {
	list, rest, ok := strings.Cut(path, "[].")
	if !ok {
		return []instance{{path, lookup(doc, path)}}
	}
	items, _ := lookup(doc, list).([]any)
	out := make([]instance, 0, len(items))
	for i, item := range items {
		out = append(out, instance{fmt.Sprintf("%s.%d.%s", list, i, rest), lookup(item, rest)})
	}
	return out
}

// outputError is the error the SDK returns for a spec it refuses before
// sending: the API's 422 shape, with every problem in Errors.
func outputError(errs []FieldError) *Error {
	return &Error{
		Type:    ErrorTypeInvalidRequest,
		Code:    "validation_failed",
		Param:   errs[0].Param,
		Message: errs[0].Message,
		Errors:  errs,
	}
}

// checkOutput refuses an incomplete whole spec before it is sent.
func checkOutput(spec *OutputSpec) error {
	if spec == nil {
		return nil
	}
	if errs := ValidateOutput(*spec); len(errs) > 0 {
		return outputError(errs)
	}
	return nil
}
