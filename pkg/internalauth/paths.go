package internalauth

const (
	// DefaultInternalJWTPublicKeyPath is the default path inside containers where the
	// internal auth public key is mounted.
	DefaultInternalJWTPublicKeyPath = "/config/internal_jwt_public.key"

	// DefaultInternalJWTPrivateKeyPath is the default path inside containers where the
	// internal auth private key is mounted.
	DefaultInternalJWTPrivateKeyPath = "/secrets/internal_jwt_private.key"
)
