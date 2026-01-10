package grpc

import "strings"

func buildS3Prefix(teamId, userId string) string {
	prefix := teamId + "/" + userId
	return strings.TrimLeft(prefix, "/")
}

