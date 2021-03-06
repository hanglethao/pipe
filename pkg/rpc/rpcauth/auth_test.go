// Copyright 2020 The PipeCD Authors.
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

package rpcauth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"
)

func TestExtractToken(t *testing.T) {
	testcases := []struct {
		name                    string
		ctx                     context.Context
		expectedCredentials     string
		expectedCredentialsType CredentialsType
		failed                  bool
	}{
		{
			name:                    "missing token",
			ctx:                     context.TODO(),
			expectedCredentials:     "",
			expectedCredentialsType: UnknownCredentials,
			failed:                  true,
		},
		{
			name: "malformed token: empty",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"authorization": []string{},
			}),
			expectedCredentials:     "",
			expectedCredentialsType: UnknownCredentials,
			failed:                  true,
		},
		{
			name: "malformed token: missing prefix",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"authorization": []string{"token"},
			}),
			expectedCredentials:     "",
			expectedCredentialsType: UnknownCredentials,
			failed:                  true,
		},
		{
			name: "malformed token: missing token",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"authorization": []string{"ID-TOKEN "},
			}),
			expectedCredentials:     "",
			expectedCredentialsType: IDTokenCredentials,
			failed:                  true,
		},
		{
			name: "should be ok with IDToken",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"authorization": []string{"ID-TOKEN token"},
			}),
			expectedCredentials:     "token",
			expectedCredentialsType: IDTokenCredentials,
			failed:                  false,
		},
		{
			name: "should be ok with PipedToken",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"authorization": []string{"PIPED-TOKEN key"},
			}),
			expectedCredentials:     "key",
			expectedCredentialsType: PipedTokenCredentials,
			failed:                  false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			creds, err := extractCredentials(tc.ctx)
			assert.Equal(t, tc.failed, err != nil)
			assert.Equal(t, tc.expectedCredentials, creds.Data)
			assert.Equal(t, tc.expectedCredentialsType, creds.Type)
		})
	}
}

func TestExtractCookie(t *testing.T) {
	testcases := []struct {
		name           string
		ctx            context.Context
		expectedCookie map[string]string
		failed         bool
	}{
		{
			name:           "missing metadata",
			ctx:            context.TODO(),
			expectedCookie: nil,
			failed:         true,
		},
		{
			name: "malformed cookie: empty",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"cookie": []string{},
			}),
			expectedCookie: nil,
			failed:         true,
		},
		{
			name: "malformed cookie: wrong format",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"cookie": []string{"token=xxx; another=yyy=zzz"},
			}),
			expectedCookie: nil,
			failed:         true,
		},
		{
			name: "ok",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				"cookie": []string{"token=xxx; another=yyy"},
			}),
			expectedCookie: map[string]string{"token": "xxx", "another": "yyy"},
			failed:         false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cookie, err := extractCookie(tc.ctx)
			assert.Equal(t, tc.failed, err != nil)
			assert.Equal(t, tc.expectedCookie, cookie)
		})
	}
}
