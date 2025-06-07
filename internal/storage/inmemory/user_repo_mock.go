package inmemory

import (
	"context"
	"fmt"
	"sync"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"gorm.io/gorm"
)

// MockUserRepository - это in-memory реализация UserRepository для тестов.
type MockUserRepository struct {
	mu     sync.RWMutex
	users  map[uint]*models.User
	nextID uint
}

// NewMockUserRepository создает новый экземпляр мок-репозитория.
func NewMockUserRepository() service.UserRepository {
	repo := &MockUserRepository{
		users:  make(map[uint]*models.User),
		nextID: 1,
	}
	// Можно добавить начальные данные, если нужно
	repo.FindOrCreateByTelegramID(context.Background(), 1, "testuser", "Test", "User")
	return repo
}

func (m *MockUserRepository) FindOrCreateByTelegramID(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, user := range m.users {
		if user.TelegramID == telegramID {
			return user, nil
		}
	}

	newUser := &models.User{
		Model:      gorm.Model{ID: m.nextID},
		TelegramID: telegramID,
		Username:   username,
		FirstName:  firstName,
		LastName:   lastName,
		IsAdmin:    true,
	}
	m.users[m.nextID] = newUser
	m.nextID++
	return newUser, nil
}

func (m *MockUserRepository) ListAll(ctx context.Context) ([]*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allUsers []*models.User
	for _, user := range m.users {
		allUsers = append(allUsers, user)
	}
	return allUsers, nil
}

func (m *MockUserRepository) FindByID(ctx context.Context, id uint) (*models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, exists := m.users[id]
	if !exists {
		return nil, fmt.Errorf("user with ID %d not found", id)
	}
	return user, nil
}
