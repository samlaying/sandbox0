package dbpool

import "testing"

func TestBuildSetSearchPathSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{
			name:   "simple schema",
			schema: "global_gateway",
			want:   `SET search_path TO "global_gateway", public`,
		},
		{
			name:   "quotes schema safely",
			schema: `schema"withquote`,
			want:   `SET search_path TO "schema""withquote", public`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildSetSearchPathSQL(tt.schema); got != tt.want {
				t.Fatalf("buildSetSearchPathSQL(%q) = %q, want %q", tt.schema, got, tt.want)
			}
		})
	}
}
