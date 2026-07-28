package store

// CookiePersisterAdapter wraps Store to implement cardigann-go's
// httpclient.CookiePersister interface (without key parameter).
// The encryption key is captured at construction time.
type CookiePersisterAdapter struct {
	store *Store
	key   []byte
}

func NewCookiePersisterAdapter(st *Store, key []byte) *CookiePersisterAdapter {
	return &CookiePersisterAdapter{store: st, key: key}
}

func (a *CookiePersisterAdapter) LoadCookies(indexerID string) (string, error) {
	return a.store.LoadCookies(indexerID, a.key)
}

func (a *CookiePersisterAdapter) SaveCookies(indexerID string, cookieJSON string) error {
	return a.store.SaveCookies(indexerID, cookieJSON, a.key)
}
