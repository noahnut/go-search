package s3

import (
	"errors"

	"github.com/noahfan/go-search/storage"
)

var _ storage.Storage = (*Store)(nil)

var errNotImplemented = errors.New("s3: not implemented")

// Store is a placeholder for an S3-backed storage backend.
// Implement Put/Get/Delete/Each using the AWS SDK when needed.
type Store struct{}

func New() *Store { return &Store{} }

func (s *Store) Put(key string, value []byte) error     { return errNotImplemented }
func (s *Store) Get(key string) ([]byte, bool)          { return nil, false }
func (s *Store) Delete(key string) error                { return errNotImplemented }
func (s *Store) Each(fn func(key string, value []byte)) {}
func (s *Store) Size() int                              { return 0 }
func (s *Store) Clear() error                           { return errNotImplemented }
func (s *Store) Close() error                           { return nil }
func (s *Store) Compact() error                         { return nil }
func (s *Store) Type() storage.StorageType              { return storage.FileStorage }
func (s *Store) Has(key string) bool                    { return false }
