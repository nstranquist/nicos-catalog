package catalog

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// fieldSpec is one frozen field of a published DTO.
type fieldSpec struct {
	Name    string
	Type    string
	JSONTag string
}

// publicEntityGolden is the frozen shape of the publication DTO.
//
// Changing this list is a deliberate privacy decision, never a routine edit.
// Adding a field means new data leaves the host, so the same change must also
// update testdata/public_projection.golden.json and SECURITY.md.
var publicEntityGolden = []fieldSpec{
	{"ID", "string", "id"},
	{"Name", "string", "name"},
	{"Kind", "string", "kind"},
	{"Status", "string", "status,omitempty"},
	{"Summary", "string", "summary,omitempty"},
	{"Tags", "[]string", "tags,omitempty"},
	{"URL", "string", "url,omitempty"},
	{"Connections", "[]catalog.PublicConnection", "connections,omitempty"},
}

// Tripwire A — the field set is ordered and exact.
//
// Order is asserted as well as membership: reordering fields changes nothing
// semantically but changes what a reviewer sees, and a closed DTO should not
// drift underneath review.
func TestPublicEntityShapeIsFrozen(t *testing.T) {
	rt := reflect.TypeOf(PublicEntity{})
	// +1 for the trailing `_ struct{}` guard that blocks unkeyed literals.
	if got, want := rt.NumField(), len(publicEntityGolden)+1; got != want {
		t.Fatalf("PublicEntity has %d fields, golden set expects %d (+1 guard).\n"+
			"Adding a field to a closed publication DTO is a deliberate privacy decision: "+
			"update publicEntityGolden AND testdata/public_projection.golden.json AND SECURITY.md "+
			"in the same change.", got, want)
	}
	for i, want := range publicEntityGolden {
		field := rt.Field(i)
		if field.Name != want.Name {
			t.Fatalf("field %d is %q, want %q (order is part of the contract)", i, field.Name, want.Name)
		}
		if got := field.Type.String(); got != want.Type {
			t.Fatalf("field %s has type %s, want %s", want.Name, got, want.Type)
		}
		if got := field.Tag.Get("json"); got != want.JSONTag {
			t.Fatalf("field %s has json tag %q, want %q (the tag is the wire contract)", want.Name, got, want.JSONTag)
		}
	}
	guard := rt.Field(rt.NumField() - 1)
	if guard.Name != "_" || guard.Type.Kind() != reflect.Struct || guard.Type.NumField() != 0 {
		t.Fatalf("last field is %s %s, want the `_ struct{}` unkeyed-literal guard", guard.Name, guard.Type)
	}
}

// Tripwire B — every type reachable from the projection is closed.
//
// This is what makes `Annotations map[string]string` structurally impossible to
// add: a map, interface, pointer, channel, function, or foreign struct anywhere
// in the reachable graph fails the test regardless of what it is named.
func TestPublicProjectionTypeGraphIsClosed(t *testing.T) {
	allowedStructs := map[string]bool{
		"catalog.PublicProjection": true,
		"catalog.PublicEntity":     true,
		"catalog.PublicConnection": true,
	}
	var walk func(rt reflect.Type, path string, depth int)
	seen := map[reflect.Type]bool{}
	walk = func(rt reflect.Type, path string, depth int) {
		if depth > 8 {
			t.Fatalf("%s: type graph deeper than expected; a closed DTO should be shallow", path)
		}
		if seen[rt] {
			return
		}
		seen[rt] = true
		switch rt.Kind() {
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int64, reflect.Float64:
			return
		case reflect.Slice:
			walk(rt.Elem(), path+"[]", depth+1)
			return
		case reflect.Struct:
			if !allowedStructs[rt.String()] {
				t.Fatalf("%s: struct %s is not part of the closed publication set", path, rt)
			}
			for i := 0; i < rt.NumField(); i++ {
				field := rt.Field(i)
				if field.Anonymous {
					t.Fatalf("%s: embedded field %s would splice a foreign type into the DTO", path, field.Name)
				}
				if field.Name == "_" {
					continue
				}
				walk(field.Type, path+"."+field.Name, depth+1)
			}
			return
		default:
			t.Fatalf("%s: kind %s is not permitted in a closed publication DTO "+
				"(maps, interfaces, pointers, channels and funcs can all carry arbitrary host data)",
				path, rt.Kind())
		}
	}
	walk(reflect.TypeOf(PublicProjection{}), "PublicProjection", 0)
}

// forbiddenInPublic names private Entity fields that must never appear on the
// publication DTO, with the reason each is withheld.
var forbiddenInPublic = map[string]string{
	"Owner":       "owner telemetry",
	"Surface":     "host-internal surface taxonomy",
	"Entrypoint":  "filesystem or command entrypoint",
	"Annotations": "arbitrary host key/value payload",
	"Refs":        "raw refs; use Connections, which is filtered to included items",
	"Visibility":  "host visibility policy",
	"PublicURL":   "raw URL; use URL, which is validated and allowlisted",
	"Description": "raw description; use Summary, which is truncated and scanned",
}

// intentionallyProjected names Entity fields that legitimately reach the DTO.
var intentionallyProjected = map[string]bool{
	"ID": true, "Name": true, "Kind": true, "Status": true, "Tags": true,
}

// Tripwire C — the denylist is derived from Entity, not from a fixture.
func TestPublicEntityCannotCarryPrivateEntityFields(t *testing.T) {
	entityType := reflect.TypeOf(Entity{})
	forbiddenTags := map[string]string{}
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		if reason, ok := forbiddenInPublic[field.Name]; ok {
			tag := strings.Split(field.Tag.Get("json"), ",")[0]
			if tag != "" {
				forbiddenTags[strings.ToLower(tag)] = reason
			}
		}
	}
	publicType := reflect.TypeOf(PublicEntity{})
	for i := 0; i < publicType.NumField(); i++ {
		field := publicType.Field(i)
		if field.Name == "_" {
			continue
		}
		if reason, ok := forbiddenInPublic[field.Name]; ok {
			t.Fatalf("PublicEntity.%s is forbidden: %s", field.Name, reason)
		}
		tag := strings.ToLower(strings.Split(field.Tag.Get("json"), ",")[0])
		if reason, ok := forbiddenTags[tag]; ok && tag != "" {
			t.Fatalf("PublicEntity.%s uses json tag %q, which is the wire name of a forbidden field: %s",
				field.Name, tag, reason)
		}
	}
}

// Companion to tripwire C: a new Entity field must be consciously classified as
// either forbidden or intentionally projected, so growing the private contract
// cannot silently bypass the review the public contract gets.
func TestForbiddenSetCoversEveryPrivateEntityField(t *testing.T) {
	entityType := reflect.TypeOf(Entity{})
	for i := 0; i < entityType.NumField(); i++ {
		field := entityType.Field(i)
		if field.Name == "_" || field.PkgPath != "" {
			continue
		}
		_, forbidden := forbiddenInPublic[field.Name]
		if !forbidden && !intentionallyProjected[field.Name] {
			t.Fatalf("Entity.%s is unclassified.\n"+
				"Every entity field must be declared either forbidden (add it to forbiddenInPublic "+
				"with a reason) or publishable (add it to intentionallyProjected). "+
				"Leaving it unclassified means nobody decided whether it may leave the host.", field.Name)
		}
	}
}

// goldenProjection builds a maximally-populated projection for the wire test.
func goldenProjection(t *testing.T) PublicProjection {
	t.Helper()
	index := Index{Entities: []Entity{
		{
			ID: "service.beta", Name: "Beta API", Kind: "service", Status: "shipped",
			Description: "Inventory and dependency search — with a unicode em dash.",
			Tags:        []string{"go", "public"}, Visibility: VisibilityPublic,
			PublicURL: "https://example.com/beta",
		},
		{
			ID: "system.alpha", Name: "Alpha Platform", Kind: "system", Status: "shipped",
			Description: "Ownership graph.", Tags: []string{"platform"},
			Visibility: VisibilityPublic, PublicURL: "https://example.com/alpha",
			Refs: []Ref{{Kind: "contains", Target: "service.beta"}},
			// Private fields, all of which must be absent from the wire form.
			Owner: "platform-team", Surface: "internal", Entrypoint: "cmd/alpha/main.go",
			Annotations: map[string]string{"query": "private sample"},
		},
	}}
	projection, err := ProjectPublic(context.Background(), index, ProjectionPolicy{
		AllowHosts: []string{"example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

// Tripwire D — the JSON key set is frozen.
//
// A tag rename is invisible to tripwires A through C in behavior terms but
// silently breaks every consumer reading the artifact by key, which is exactly
// how a downstream page breaks at runtime rather than at compile time.
func TestPublicProjectionJSONKeysAreFrozen(t *testing.T) {
	payload, err := json.Marshal(goldenProjection(t))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		SchemaVersion int                      `json:"schema_version"`
		Items         []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != SchemaVersion {
		t.Fatalf("schema_version = %d, want %d", decoded.SchemaVersion, SchemaVersion)
	}
	if len(decoded.Items) != 2 {
		t.Fatalf("golden projection produced %d items, want 2", len(decoded.Items))
	}
	allowedKeys := map[string]bool{}
	for _, spec := range publicEntityGolden {
		allowedKeys[strings.Split(spec.JSONTag, ",")[0]] = true
	}
	for _, item := range decoded.Items {
		for key := range item {
			if !allowedKeys[key] {
				t.Fatalf("projected item carries unexpected wire key %q; "+
					"the JSON tag set is part of the published contract", key)
			}
		}
	}
	// The fully-populated entity must round-trip every publishable key.
	var populated map[string]interface{}
	for _, item := range decoded.Items {
		if item["id"] == "system.alpha" {
			populated = item
		}
	}
	if populated == nil {
		t.Fatal("golden projection lost system.alpha")
	}
	for _, key := range []string{"id", "name", "kind", "status", "summary", "tags", "url", "connections"} {
		if _, ok := populated[key]; !ok {
			t.Fatalf("fully-populated item is missing wire key %q", key)
		}
	}
}

// Tripwire E — property-level, fixture-independent leak check.
func FuzzProjectPublicNeverLeaksPrivateFields(f *testing.F) {
	f.Add("alpha", "Alpha", "system", "a summary")
	f.Add("b.c-d_e", "Name", "service", "")
	f.Add("z9", "Ünïcødé", "kind", "  padded  ")
	const sentinel = "ZZPRIVATEZZ"
	f.Fuzz(func(t *testing.T, id, name, kind, description string) {
		entity := Entity{
			ID: id, Name: name, Kind: kind, Description: description,
			Visibility: VisibilityPublic,
			// Every private field carries the sentinel.
			Owner:       sentinel,
			Surface:     sentinel,
			Entrypoint:  sentinel,
			Annotations: map[string]string{sentinel: sentinel},
		}
		if err := validateEntity(normalizeEntity(entity)); err != nil {
			t.Skip()
		}
		projection, err := ProjectPublic(context.Background(),
			Index{Entities: []Entity{normalizeEntity(entity)}}, ProjectionPolicy{})
		if err != nil {
			return
		}
		payload, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), sentinel) {
			t.Fatalf("private field reached the public projection: %s", payload)
		}
	})
}

// BuildInfo and DriftReport are embedded by value into downstream receipts, so
// their shapes are contracts too.
func TestBuildInfoShapeIsFrozen(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(BuildInfo{}), []string{
		"version", "commit,omitempty", "modified,omitempty", "schema_version", "capabilities",
	})
}

func TestDriftReportShapeIsFrozen(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(DriftReport{}), []string{
		"ok", "changed", "reason,omitempty", "expected_digest,omitempty", "actual_digest,omitempty",
	})
}

func assertJSONTags(t *testing.T, rt reflect.Type, want []string) {
	t.Helper()
	var got []string
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if field.Name == "_" {
			continue
		}
		got = append(got, field.Tag.Get("json"))
	}
	if len(got) != len(want) {
		t.Fatalf("%s has %d exported fields %v, want %d %v", rt, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s field %d json tag = %q, want %q", rt, i, got[i], want[i])
		}
	}
}
