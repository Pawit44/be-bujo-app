package service

import (
	"errors"

	"bujo/internal/models"
	"bujo/internal/repository"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrInvalidRole      = errors.New("role must be \"user\" or \"admin\"")
	ErrCannotDemoteLast = errors.New("cannot demote the last admin")
	ErrCannotDeleteLast = errors.New("cannot delete the last admin")
	ErrCannotDeleteSelf = errors.New("cannot delete your own account here")
)

// AdminService manages accounts only — it never touches another user's
// journal content, by design (see CollectionRepository / EntryRepository,
// which AdminService deliberately does not depend on).
type AdminService struct {
	users    repository.UserRepository
	sessions repository.SessionRepository
}

func NewAdminService(users repository.UserRepository, sessions repository.SessionRepository) *AdminService {
	return &AdminService{users: users, sessions: sessions}
}

func (s *AdminService) ListUsers() ([]models.User, error) {
	return s.users.List()
}

// UpdateRole promotes or demotes an account. Refuses to demote the last
// remaining admin, so nobody can lock everyone (including themselves) out
// of user management.
func (s *AdminService) UpdateRole(targetID uint, role models.Role) (*models.User, error) {
	if role != models.RoleUser && role != models.RoleAdmin {
		return nil, ErrInvalidRole
	}
	target, err := s.users.FindByID(targetID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if target.Role == models.RoleAdmin && role == models.RoleUser {
		admins, err := s.users.CountByRole(models.RoleAdmin)
		if err != nil {
			return nil, err
		}
		if admins <= 1 {
			return nil, ErrCannotDemoteLast
		}
	}

	target.Role = role
	if err := s.users.Update(target); err != nil {
		return nil, err
	}
	// Force re-login everywhere so the role change (and any UI it gates)
	// takes effect immediately instead of at the old session's next renewal.
	_ = s.sessions.DeleteAllForUser(target.ID)
	return target, nil
}

// DeleteUser removes the account and every journal entry/collection it
// owns. Refuses to delete the actor's own account (self-service deletion is
// AuthService.DeleteAccount) or the last admin, so the admin panel can
// never delete its own only door.
func (s *AdminService) DeleteUser(actorID, targetID uint) error {
	if actorID == targetID {
		return ErrCannotDeleteSelf
	}
	target, err := s.users.FindByID(targetID)
	if err != nil {
		return ErrUserNotFound
	}
	if target.Role == models.RoleAdmin {
		admins, err := s.users.CountByRole(models.RoleAdmin)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrCannotDeleteLast
		}
	}
	return s.users.DeleteCascade(target)
}
