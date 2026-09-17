// Copyright 2026 Sayak Mukhopadhyay
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseGitHubAllowedInstallationIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    []int64
		wantErr string
	}{
		{name: "one", value: "123", want: []int64{123}},
		{name: "multiple with whitespace", value: " 123, 456 ,789 ", want: []int64{123, 456, 789}},
		{name: "missing", wantErr: "is required"},
		{name: "empty entry", value: "123,,456", wantErr: "comma-separated positive integers"},
		{name: "not decimal", value: "123,abc", wantErr: "invalid installation ID"},
		{name: "zero", value: "0", wantErr: "invalid installation ID"},
		{name: "negative", value: "-1", wantErr: "invalid installation ID"},
		{name: "duplicate", value: "123,123", wantErr: "duplicate installation ID 123"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseGitHubAllowedInstallationIDs(test.value)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("parseGitHubAllowedInstallationIDs() error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGitHubAllowedInstallationIDs() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseGitHubAllowedInstallationIDs() = %v, want %v", got, test.want)
			}
		})
	}
}
