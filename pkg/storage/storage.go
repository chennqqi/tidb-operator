package storage

import (
	"context"
	"errors"
	"time"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

const (
	// Schema like mysql schema
	Schema = "/tk"

	// storageTimeout data storage timeout
	storageTimeout = 3 * time.Second
)

var (
	// ErrNoImplement 接口没有实现类错误
	ErrNoImplement = errors.New("implementation doesn't exist")
	// ErrCreateEtcdStorage 创建etcd storage失败
	ErrCreateEtcdStorage = errors.New("cannt creat etcd storage")
	// ErrNoNode is returned by functions to specify the requested
	// resource does not exist.
	ErrNoNode = errors.New("node doesn't exist")
	// ErrTimeout is returned by functions that wait for a result
	// when the timeout value is reached.
	ErrTimeout = errors.New("deadline exceeded")
	// ErrInterrupted is returned by functions that wait for a result
	// when they are interrupted.
	ErrInterrupted = errors.New("interrupted")
	// ErrBadVersion is returned by an update function that
	// failed to update the data because the version was different
	ErrBadVersion = errors.New("bad node version")
)

// Impl 封装底层storage, 所有的storage必须实现该接口
type Impl interface {
	Close() error
	List(ctx context.Context) ([]string, error)
	ListKey(ctx context.Context, prefix string) ([]string, error)
	// Create creates the initial version of a path.
	Create(ctx context.Context, path string, contents []byte) (Version, error)

	// Delete will never be called on a directory.
	// Returns ErrNodeExists if the path doesn't exist.
	// Returns ErrBadVersion if the provided version is not current.
	Delete(ctx context.Context, path string, version Version) error
	DeleteAll(ctx context.Context, path string) error
	// Update updates path
	Update(ctx context.Context, path string, contents []byte, version Version) (Version, error)
	// Get returns the content and version of a path.
	Get(ctx context.Context, path string) ([]byte, Version, error)
}

// Version is an interface that describes a key version.
type Version interface {
	// String returns a text representation of the version.
	String() string
}

// Storage 数据存储接口
type Storage struct {
	Impl
}

// IsNil 返回Storage是否被初始化

// Create 保存key/value
func (s *Storage) Create(key string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), storageTimeout)
	defer cancel()
	_, err := s.Impl.Create(ctx, key, contents)
	return err
}

// Get 获取指定key的data
func (s *Storage) Get(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), storageTimeout)
	defer cancel()
	data, _, err := s.Impl.Get(ctx, key)
	return data, err
}

// List all keys
func (s *Storage) List() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), storageTimeout)
	defer cancel()
	return s.Impl.List(ctx)
}

// ListKey 返回指定path下的key
func (s *Storage) ListKey(prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), storageTimeout)
	defer cancel()
	return s.Impl.ListKey(ctx, prefix)
}

// Delete 删除指定的key
func (s *Storage) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), storageTimeout)
	defer cancel()
	return s.Impl.Delete(ctx, key, nil)
}

// Update 保存key/value
func (s *Storage) Update(key string, contents []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), storageTimeout)
	defer cancel()
	_, err := s.Impl.Update(ctx, key, contents, nil)
	return err
}

// Factory Impl工厂
type Factory func(serverAddr, name string) (Impl, error)

var (
	factories = make(map[string]Factory)
)

// RegisterStorageFactory factory注册函数
func RegisterStorageFactory(name string, factory Factory) {
	if factories[name] != nil {
		logs.Error("Duplicate store.Factory registration for %v", name)
	}
	factories[name] = factory
}

// NewStorage 返回一个指定实现的storage
func NewStorage(implementation, serverAddress, name string) (Storage, error) {
	factory, ok := factories[implementation]
	if !ok {
		return Storage{}, ErrNoImplement
	}

	impl, err := factory(serverAddress, name)
	if err != nil {
		return Storage{}, err
	}
	return Storage{impl}, nil
}

// NewDefaultStorage new default storage
func NewDefaultStorage(name, etcdAddress string) (Storage, error) {
	st := beego.AppConfig.String("storage")
	if len(st) == 0 {
		st = "etcd"
	}
	logs.Info("Create %s[%s] %s storage", st, etcdAddress, name)
	return NewStorage(st, etcdAddress, name)
}
