package main

import "testing"

func TestSQLParametersPreserveLiterals(t *testing.T) {
	query := `SELECT '?' AS texto, ? AS um, 'it''s ?' AS literal, ? AS dois`
	expected := `SELECT '?' AS texto, $1 AS um, 'it''s ?' AS literal, $2 AS dois`
	if got := sqlParameters(query, true); got != expected {
		t.Fatalf("SQL: %s", got)
	}
	if got := sqlParameters(query, false); got != query {
		t.Fatal("consulta SQLite alterada")
	}
}
