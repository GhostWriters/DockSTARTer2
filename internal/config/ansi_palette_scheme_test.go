package config

import "testing"

func TestParseBase16SchemeMetaDescription(t *testing.T) {
	data := []byte("system: \"base24\"\nname: \"Alucard\"\nauthor: \"a\"\ndescription: \"Alucard Classic - Dracula light\"\nvariant: \"light\"\npalette:\n  base00: \"#fffbeb\"\n")
	meta, err := ParseBase16SchemeMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Description != "Alucard Classic - Dracula light" {
		t.Errorf("Description = %q", meta.Description)
	}
}
