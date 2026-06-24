// Copyright 2025 Microsoft Corporation
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

package imagemirror

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/containers/azcontainerregistry"
)

// exchangeACRAccessToken exchanges an ARM access token for an ACR refresh token.
func exchangeACRAccessToken(ctx context.Context, armToken azcore.AccessToken, acrFQDN string) (azcore.AccessToken, error) {
	endpoint, err := url.Parse(fmt.Sprintf("https://%s", acrFQDN))
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("failed to parse ACR endpoint: %w", err)
	}

	client, err := azcontainerregistry.NewAuthenticationClient(endpoint.String(), nil)
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("failed to create ACR authentication client: %w", err)
	}

	refreshResponse, err := client.ExchangeAADAccessTokenForACRRefreshToken(ctx, azcontainerregistry.PostContentSchemaGrantTypeAccessToken, endpoint.Hostname(), &azcontainerregistry.AuthenticationClientExchangeAADAccessTokenForACRRefreshTokenOptions{
		AccessToken: &armToken.Token,
	})
	if err != nil {
		return azcore.AccessToken{}, fmt.Errorf("failed to exchange AAD access token for ACR refresh token: %w", err)
	}

	if refreshResponse.RefreshToken == nil {
		return azcore.AccessToken{}, errors.New("got an empty response when exchanging AAD access token for ACR refresh token")
	}

	accessToken := *refreshResponse.RefreshToken

	// Parse the JWT to extract the expiry, so we can report it.
	parts := splitJWT(accessToken)
	if len(parts) != 3 {
		// If we can't parse the token, just set a reasonable default expiry.
		return azcore.AccessToken{
			Token:     accessToken,
			ExpiresOn: time.Now().Add(1 * time.Hour),
		}, nil
	}

	claims, err := parseJWTClaims(parts[1])
	if err != nil {
		return azcore.AccessToken{
			Token:     accessToken,
			ExpiresOn: time.Now().Add(1 * time.Hour),
		}, nil
	}

	expiry := extractExpiry(claims)
	return azcore.AccessToken{
		Token:     accessToken,
		ExpiresOn: expiry,
	}, nil
}

// exchangeACRAccessTokenWithRetry wraps exchangeACRAccessToken with exponential backoff retry.
func exchangeACRAccessTokenWithRetry(ctx context.Context, cred azcore.TokenCredential, acrFQDN string) (azcore.AccessToken, error) {
	var acrToken azcore.AccessToken
	var lastErr error
	backoff := wait.Backoff{
		Steps:    5,
		Duration: 2 * time.Second,
		Factor:   2.0,
		Jitter:   0.1,
	}

	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		armToken, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://management.azure.com/.default"},
		})
		if err != nil {
			return false, fmt.Errorf("failed to get ARM token: %w", err)
		}

		acrToken, err = exchangeACRAccessToken(ctx, armToken, acrFQDN)
		if err != nil {
			lastErr = err
			// Retry on failure.
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		if lastErr != nil {
			return azcore.AccessToken{}, fmt.Errorf("failed to exchange ACR access token after retries: %w: last exchange error: %w", err, lastErr)
		}
		return azcore.AccessToken{}, fmt.Errorf("failed to exchange ACR access token after retries: %w", err)
	}

	return acrToken, nil
}

// splitJWT splits a JWT into its three parts without importing a JWT library.
func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := range token {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}

// parseJWTClaims base64-decodes and parses the claims section of a JWT.
func parseJWTClaims(encoded string) (map[string]any, error) {
	// JWT uses base64url encoding without padding.
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// extractExpiry extracts the "exp" claim from JWT claims as a time.Time.
func extractExpiry(claims map[string]any) time.Time {
	switch exp := claims["exp"].(type) {
	case float64:
		return time.Unix(int64(exp), 0)
	case json.Number:
		timestamp, _ := exp.Int64()
		return time.Unix(timestamp, 0)
	default:
		return time.Now().Add(1 * time.Hour)
	}
}
