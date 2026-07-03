package s3

import "github.com/noahfan/go-search/storage"

var _ storage.Storage = (*Store)(nil)

type Store struct{}

func New(path string) (*Store, error) // opens or creates the log file

// Store implements storage.Storage
func (s *Store) Put(key, value string) error
func (s *Store) Get(key string) (string, bool)
func (s *Store) Delete(key string) error
func (s *Store) Each(fn func(key, value string))
func (s *Store) Size() int
func (s *Store) Close() error

// Compact rewrites the log keeping only live keys, shrinking the file.
// Called by the user or the persistence manager after a snapshot.
func (s *Store) Compact() error
