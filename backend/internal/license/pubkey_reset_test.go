package license

import "sync"

// resetPubKeyCache forces the sync.Once for VendorPublicKey() to fire
// again on the next call. Only used inside tests where we swap public
// keys between assertions.
func resetPubKeyCache() {
	pubKeyOnce = sync.Once{}
	pubKey = nil
	pubKeyErr = nil
}
