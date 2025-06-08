package gorm

import (
	"context"
	"errors"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"gorm.io/gorm"
)

type GormUserRepository struct {
	db *gorm.DB
}

func NewGormUserRepository(db *gorm.DB) (service.UserRepository, error) {
	return &GormUserRepository{db: db}, nil
}

func (r *GormUserRepository) FindOrCreateByTelegramID(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where(models.User{TelegramID: telegramID}).First(&user).Error
	if err == nil {
		if user.Username != username || user.FirstName != firstName || user.LastName != lastName {
			user.Username = username
			user.FirstName = firstName
			user.LastName = lastName
			if err := r.db.WithContext(ctx).Save(&user).Error; err != nil {
				return nil, err
			}
		}
		return &user, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	newUser := &models.User{
		TelegramID: telegramID,
		Username:   username,
		FirstName:  firstName,
		LastName:   lastName,
	}

	if err := r.db.WithContext(ctx).Create(newUser).Error; err != nil {
		return nil, err
	}
	return newUser, nil
}

func (r *GormUserRepository) ListAll(ctx context.Context) ([]*models.User, error) {
	var users []*models.User
	err := r.db.WithContext(ctx).Find(&users).Error
	return users, err
}

func (r *GormUserRepository) FindByID(ctx context.Context, id uint) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	return &user, err
}
