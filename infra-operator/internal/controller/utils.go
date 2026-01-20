/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"crypto/rand"
	"encoding/base64"
)

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	// Generate random bytes
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a default string if random generation fails
		return "defaultsecret123456789012"
	}

	// Encode to base64 and truncate to desired length
	encoded := base64.URLEncoding.EncodeToString(bytes)
	if len(encoded) > length {
		return encoded[:length]
	}
	return encoded
}
