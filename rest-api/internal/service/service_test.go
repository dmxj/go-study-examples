package service

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
	"github.com/dmxj/go-study-examples/rest-api/internal/model"
	"github.com/dmxj/go-study-examples/rest-api/internal/repository"
)

// fakeBookRepo 是 BookRepository 的内存实现，用于在不启动数据库的情况下测试业务逻辑。
type fakeBookRepo struct {
	books  map[uint]*model.Book
	nextID uint
	last   repository.BookFilter // 记录最近一次 List 的参数
}

func newFakeBookRepo() *fakeBookRepo { return &fakeBookRepo{books: map[uint]*model.Book{}} }

func (f *fakeBookRepo) Create(_ context.Context, b *model.Book) error {
	for _, x := range f.books {
		if x.ISBN == b.ISBN {
			return apperr.ErrConflict
		}
	}
	f.nextID++
	b.ID = f.nextID
	cp := *b
	f.books[b.ID] = &cp
	return nil
}

func (f *fakeBookRepo) GetByID(_ context.Context, id uint) (*model.Book, error) {
	b, ok := f.books[id]
	if !ok {
		return nil, apperr.ErrNotFound
	}
	cp := *b
	return &cp, nil
}

func (f *fakeBookRepo) Update(_ context.Context, b *model.Book) error {
	cp := *b
	f.books[b.ID] = &cp
	return nil
}

func (f *fakeBookRepo) Delete(_ context.Context, id uint) error {
	if _, ok := f.books[id]; !ok {
		return apperr.ErrNotFound
	}
	delete(f.books, id)
	return nil
}

func (f *fakeBookRepo) List(_ context.Context, flt repository.BookFilter) ([]model.Book, int64, error) {
	f.last = flt
	return nil, int64(len(f.books)), nil
}

func TestBookServiceCreateNormalizesISBN(t *testing.T) {
	svc := NewBookService(newFakeBookRepo())
	b, err := svc.Create(context.Background(), 7, model.CreateBookRequest{Title: "t", Author: "a", ISBN: "978-7-111 55842-2"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ISBN != "9787111558422" || b.CreatedBy != 7 {
		t.Errorf("Create = %+v，期望 ISBN 去掉连字符、CreatedBy=7", b)
	}
	_, err = svc.Create(context.Background(), 7, model.CreateBookRequest{Title: "t2", Author: "a", ISBN: "9787111558422"})
	if !errors.Is(err, apperr.ErrConflict) {
		t.Errorf("重复 ISBN 应返回 ErrConflict（可被 errors.Is 识别），实际 %v", err)
	}
}

func TestBookServiceUpdatePartial(t *testing.T) {
	repo := newFakeBookRepo()
	svc := NewBookService(repo)
	b, _ := svc.Create(context.Background(), 1, model.CreateBookRequest{Title: "old", Author: "a", ISBN: "9787111558422", Price: 100, Stock: 5})

	zero := 0
	got, err := svc.Update(context.Background(), b.ID, model.UpdateBookRequest{Stock: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != 0 || got.Title != "old" || got.Price != 100 {
		t.Errorf("部分更新结果 %+v，期望只有 Stock 变为 0", got)
	}
	if _, err := svc.Update(context.Background(), 999, model.UpdateBookRequest{}); !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("更新不存在的图书应返回 ErrNotFound，实际 %v", err)
	}
}

func TestBookServiceListDefaults(t *testing.T) {
	repo := newFakeBookRepo()
	svc := NewBookService(repo)
	page, err := svc.List(context.Background(), model.ListBooksQuery{Page: 3})
	if err != nil {
		t.Fatal(err)
	}
	if repo.last.Offset != 20 || repo.last.Limit != 10 || repo.last.Sort != "-id" {
		t.Errorf("默认分页参数错误: %+v", repo.last)
	}
	if page.Page != 3 || page.PageSize != 10 {
		t.Errorf("返回的分页信息 %+v 不正确", page)
	}
}

// fakeUserRepo 与 fakeIssuer 用于测试 AuthService。
type fakeUserRepo struct{ users map[string]*model.User }

func (f *fakeUserRepo) Create(_ context.Context, u *model.User) error {
	if _, ok := f.users[u.Username]; ok {
		return apperr.ErrConflict
	}
	u.ID = uint(len(f.users) + 1)
	f.users[u.Username] = u
	return nil
}
func (f *fakeUserRepo) GetByUsername(_ context.Context, name string) (*model.User, error) {
	if u, ok := f.users[name]; ok {
		return u, nil
	}
	return nil, apperr.ErrNotFound
}
func (f *fakeUserRepo) GetByID(_ context.Context, id uint) (*model.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, apperr.ErrNotFound
}

type fakeIssuer struct{}

func (fakeIssuer) Generate(id uint, name string) (string, error) { return "token-for-" + name, nil }

func TestAuthService(t *testing.T) {
	repo := &fakeUserRepo{users: map[string]*model.User{}}
	svc := NewAuthService(repo, fakeIssuer{}).WithBcryptCost(bcrypt.MinCost)
	ctx := context.Background()

	u, err := svc.Register(ctx, model.RegisterRequest{Username: "alice", Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash == "password123" || u.PasswordHash == "" {
		t.Fatal("密码必须以哈希形式存储")
	}

	tok, err := svc.Login(ctx, model.LoginRequest{Username: "alice", Password: "password123"})
	if err != nil || tok != "token-for-alice" {
		t.Errorf("登录 = %q, %v", tok, err)
	}
	// 用户不存在与密码错误返回同一种错误，防止用户名枚举
	for _, req := range []model.LoginRequest{
		{Username: "alice", Password: "bad-password"},
		{Username: "ghost", Password: "password123"},
	} {
		if _, err := svc.Login(ctx, req); !errors.Is(err, apperr.ErrInvalidCredentials) {
			t.Errorf("Login(%s) 应返回 ErrInvalidCredentials，实际 %v", req.Username, err)
		}
	}
}
