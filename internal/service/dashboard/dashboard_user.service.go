package dashboard

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/krobus00/hft-service/internal/entity"
	"github.com/krobus00/hft-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDashboardUserNotFound      = errors.New("user not found")
	ErrDashboardUserConflict      = errors.New("username already exists")
	ErrDashboardUserInvalidInput  = errors.New("invalid user input")
	ErrDashboardUserCannotDelete  = errors.New("cannot delete own account")
	ErrDashboardUserCannotDisable = errors.New("cannot disable own account")
)

type DashboardUserService struct {
	repo *repository.DashboardAuthRepository
}

type ListUsersInput struct {
	Search   string
	Page     int
	PageSize int
}

type CreateUserInput struct {
	Username string
	Password string
	Role     string
	IsActive bool
}

type UpdateUserInput struct {
	Username *string
	Password *string
	Role     *string
	IsActive *bool
}

func NewDashboardUserService(repo *repository.DashboardAuthRepository) *DashboardUserService {
	return &DashboardUserService{repo: repo}
}

func (s *DashboardUserService) ListUsers(ctx context.Context, input ListUsersInput) ([]entity.DashboardUser, int64, error) {
	return s.repo.ListUsers(ctx, repository.DashboardUserListFilter{
		Search:   strings.TrimSpace(input.Search),
		Page:     input.Page,
		PageSize: input.PageSize,
	})
}

func (s *DashboardUserService) GetUserByID(ctx context.Context, userID string) (*entity.DashboardUser, error) {
	user, err := s.repo.GetUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrDashboardUserNotFound
	}

	return user, nil
}

func (s *DashboardUserService) CreateUser(ctx context.Context, input CreateUserInput) (*entity.DashboardUser, error) {
	username := strings.TrimSpace(input.Username)
	role := normalizeRole(input.Role)

	if username == "" || strings.TrimSpace(input.Password) == "" || role == "" {
		return nil, ErrDashboardUserInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	user := &entity.DashboardUser{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		IsActive:     input.IsActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrDashboardUserConflict
		}

		return nil, err
	}

	created, err := s.repo.GetUserByID(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *DashboardUserService) UpdateUser(ctx context.Context, actorUserID string, userID string, input UpdateUserInput) (*entity.DashboardUser, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrDashboardUserInvalidInput
	}

	_, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrDashboardUserNotFound
	}

	patch := repository.DashboardUserPatch{}
	hasChange := false

	if input.Username != nil {
		username := strings.TrimSpace(*input.Username)
		if username == "" {
			return nil, ErrDashboardUserInvalidInput
		}
		patch.Username = &username
		hasChange = true
	}

	if input.Password != nil {
		password := strings.TrimSpace(*input.Password)
		if password == "" {
			return nil, ErrDashboardUserInvalidInput
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, hashErr
		}
		hashString := string(hash)
		patch.PasswordHash = &hashString
		hasChange = true
	}

	if input.Role != nil {
		role := normalizeRole(*input.Role)
		if role == "" {
			return nil, ErrDashboardUserInvalidInput
		}
		patch.Role = &role
		hasChange = true
	}

	if input.IsActive != nil {
		if strings.TrimSpace(actorUserID) != "" && actorUserID == userID && !*input.IsActive {
			return nil, ErrDashboardUserCannotDisable
		}
		patch.IsActive = input.IsActive
		hasChange = true
	}

	if !hasChange {
		return nil, ErrDashboardUserInvalidInput
	}

	if err := s.repo.UpdateUser(ctx, userID, patch, time.Now().UTC()); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrDashboardUserConflict
		}
		return nil, err
	}

	updated, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return updated, nil
}

func (s *DashboardUserService) DeleteUser(ctx context.Context, actorUserID string, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrDashboardUserInvalidInput
	}

	if strings.TrimSpace(actorUserID) != "" && actorUserID == userID {
		return ErrDashboardUserCannotDelete
	}

	if _, err := s.repo.GetUserByID(ctx, userID); err != nil {
		return ErrDashboardUserNotFound
	}

	return s.repo.DeleteUser(ctx, userID)
}

func normalizeRole(raw string) string {
	role := strings.ToLower(strings.TrimSpace(raw))
	switch role {
	case string(entity.DashboardRoleAdmin), string(entity.DashboardRoleUser):
		return role
	default:
		return ""
	}
}
