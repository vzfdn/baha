package main

import (
	"strings"
	"testing"
)

func TestParseHoldings(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		want        []Holding
		wantErr     bool
		errContains []string
	}{
		{
			name: "empty argument",
			args: []string{""},
			want: []Holding{},
		},
		{
			name: "whitespace-only argument",
			args: []string{"   "},
			want: []Holding{},
		},
		{
			name: "asset without amount",
			args: []string{"bitcoin"},
			want: []Holding{
				{name: "bitcoin"},
			},
		},
		{
			name: "asset with amount",
			args: []string{"bitcoin=0.002"},
			want: []Holding{
				{name: "bitcoin", amount: 0.002, hasAmount: true},
			},
		},
		{
			name: "trims whitespace",
			args: []string{"  bitcoin = 0.002  "},
			want: []Holding{
				{name: "bitcoin", amount: 0.002, hasAmount: true},
			},
		},
		{
			name: "fractional amount",
			args: []string{"bitcoin=0.000001"},
			want: []Holding{
				{name: "bitcoin", amount: 0.000001, hasAmount: true},
			},
		},
		{
			name:        "case insensitive duplicate",
			args:        []string{"bitcoin", "BITCOIN"},
			wantErr:     true,
			errContains: []string{"duplicate entry"},
		},
		{
			name:        "empty asset name",
			args:        []string{"=2"},
			wantErr:     true,
			errContains: []string{"asset name cannot be empty"},
		},
		{
			name:        "empty amount",
			args:        []string{"bitcoin="},
			wantErr:     true,
			errContains: []string{"parsing amount"},
		},
		{
			name:        "invalid amount",
			args:        []string{"bitcoin=abc"},
			wantErr:     true,
			errContains: []string{"parsing amount"},
		},
		{
			name:        "negative amount",
			args:        []string{"bitcoin=-1"},
			wantErr:     true,
			errContains: []string{"finite, non-negative"},
		},
		{
			name:        "NaN amount",
			args:        []string{"bitcoin=NaN"},
			wantErr:     true,
			errContains: []string{"finite, non-negative"},
		},
		{
			name:        "infinite amount",
			args:        []string{"bitcoin=Inf"},
			wantErr:     true,
			errContains: []string{"finite, non-negative"},
		},
		{
			name: "multiple holdings",
			args: []string{"bitcoin=0.002", "gold", "euro=10"},
			want: []Holding{
				{name: "bitcoin", amount: 0.002, hasAmount: true},
				{name: "gold"},
				{name: "euro", amount: 10, hasAmount: true},
			},
		},
		{
			name:    "multiple invalid arguments",
			args:    []string{"=2", "bitcoin=-1", "euro=abc"},
			wantErr: true,
			errContains: []string{
				"asset name cannot be empty",
				"finite, non-negative",
				"parsing amount",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHoldings(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("parseHoldings() error = nil, want error")
				}
				for _, want := range tt.errContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q, want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHoldings() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseHoldings() returned %d holdings, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("holding[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bitcoin", "bitcoin"},
		{"BITCOIN", "bitcoin"},
		{"  Bitcoin  ", "bitcoin"},
		{"  BITCOIN  ", "bitcoin"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
