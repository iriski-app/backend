package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/iriski-app/backend/internal/storage/models"
	"gorm.io/gorm"
)

func TestAddBasketItem(t *testing.T) {
	lookupErr := errors.New("basket lookup failed")
	createErr := errors.New("basket creation failed")
	itemErr := errors.New("item creation failed")
	tests := []struct {
		name       string
		lookupErr  error
		createErr  error
		itemErr    error
		wantCreate bool
		wantItem   bool
		wantErr    error
	}{
		{name: "existing basket", wantItem: true},
		{name: "new basket", lookupErr: gorm.ErrRecordNotFound, wantCreate: true, wantItem: true},
		{
			name: "wrapped not found", lookupErr: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound),
			wantCreate: true, wantItem: true,
		},
		{name: "lookup error", lookupErr: lookupErr, wantErr: lookupErr},
		{
			name: "creation error", lookupErr: gorm.ErrRecordNotFound, createErr: createErr,
			wantCreate: true, wantErr: createErr,
		},
		{name: "item error", itemErr: itemErr, wantItem: true, wantErr: itemErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sessionID := uuid.New()
			basket := models.Basket{ID: uuid.New(), SessionID: sessionID}
			wantItem := models.BasketItem{
				ID: uuid.New(), BasketID: basket.ID, InstrumentID: uuid.New(),
				Side: models.DealSideSell, Quantity: 5,
			}
			lookupCalls, createCalls, itemCalls := 0, 0, 0
			store := &fakeStore{
				GetBasketBySessionIDFunc: func(gotCtx context.Context, id uuid.UUID) (models.Basket, error) {
					lookupCalls++
					if gotCtx != ctx || id != sessionID {
						t.Error("basket lookup received incorrect context or session ID")
					}
					if tt.lookupErr != nil {
						return models.Basket{}, tt.lookupErr
					}
					return basket, nil
				},
			}
			if tt.wantCreate {
				store.CreateBasketFunc = func(gotCtx context.Context, id uuid.UUID) (models.Basket, error) {
					createCalls++
					if gotCtx != ctx || id != sessionID {
						t.Error("basket creation received incorrect context or session ID")
					}
					if tt.createErr != nil {
						return models.Basket{}, tt.createErr
					}
					return basket, nil
				}
			}
			if tt.wantItem {
				store.CreateBasketItemFunc = func(gotCtx context.Context, isin string, side models.DealSide, quantity int64, basketID uuid.UUID) (models.BasketItem, error) {
					itemCalls++
					if gotCtx != ctx || isin != "RU000A0JX0J2" || side != wantItem.Side || quantity != wantItem.Quantity || basketID != basket.ID {
						t.Error("item creation received incorrect position or basket")
					}
					if tt.itemErr != nil {
						return models.BasketItem{}, tt.itemErr
					}
					return wantItem, nil
				}
			}

			item, err := NewService(store).AddBasketItem(ctx, sessionID, "RU000A0JX0J2", models.DealSideSell, 5)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if !reflect.DeepEqual(item, wantItem) {
					t.Errorf("item = %+v, want %+v", item, wantItem)
				}
			} else if !reflect.DeepEqual(item, models.BasketItem{}) {
				t.Error("failed addition must return an empty item")
			}
			wantCreateCalls, wantItemCalls := 0, 0
			if tt.wantCreate {
				wantCreateCalls = 1
			}
			if tt.wantItem {
				wantItemCalls = 1
			}
			if lookupCalls != 1 || createCalls != wantCreateCalls || itemCalls != wantItemCalls {
				t.Errorf("calls: lookup=%d, create=%d, item=%d; want 1, %d, %d", lookupCalls, createCalls, itemCalls, wantCreateCalls, wantItemCalls)
			}
		})
	}
}

func TestGetBasket(t *testing.T) {
	lookupErr := errors.New("basket lookup failed")
	itemsErr := errors.New("items lookup failed")
	tests := []struct {
		name      string
		empty     bool
		lookupErr error
		itemsErr  error
		wantErr   error
	}{
		{name: "basket with items"},
		{name: "empty basket", empty: true},
		{name: "basket not found", lookupErr: gorm.ErrRecordNotFound, wantErr: gorm.ErrRecordNotFound},
		{name: "lookup error", lookupErr: lookupErr, wantErr: lookupErr},
		{name: "items error", itemsErr: itemsErr, wantErr: itemsErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sessionID := uuid.New()
			wantBasket := models.Basket{ID: uuid.New(), SessionID: sessionID}
			wantItems := []models.BasketItem{
				{ID: uuid.New(), BasketID: wantBasket.ID, InstrumentID: uuid.New(), Side: models.DealSideBuy, Quantity: 10},
				{ID: uuid.New(), BasketID: wantBasket.ID, InstrumentID: uuid.New(), Side: models.DealSideSell, Quantity: 5},
			}
			if tt.empty {
				wantItems = []models.BasketItem{}
			}
			itemsCalls := 0
			store := &fakeStore{
				GetBasketBySessionIDFunc: func(gotCtx context.Context, id uuid.UUID) (models.Basket, error) {
					if gotCtx != ctx || id != sessionID {
						t.Error("basket lookup received incorrect context or session ID")
					}
					if tt.lookupErr != nil {
						return models.Basket{}, tt.lookupErr
					}
					return wantBasket, nil
				},
			}
			if tt.lookupErr == nil {
				store.GetBasketItemsFunc = func(gotCtx context.Context, id uuid.UUID) ([]models.BasketItem, error) {
					itemsCalls++
					if gotCtx != ctx || id != wantBasket.ID {
						t.Error("items lookup received incorrect context or basket ID")
					}
					if tt.itemsErr != nil {
						return nil, tt.itemsErr
					}
					return wantItems, nil
				}
			}

			basket, items, err := NewService(store).GetBasket(ctx, sessionID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if !reflect.DeepEqual(basket, wantBasket) || !reflect.DeepEqual(items, wantItems) {
					t.Error("returned basket or items differ from stored values")
				}
			} else if !reflect.DeepEqual(basket, models.Basket{}) || len(items) != 0 {
				t.Error("failed lookup must return an empty basket and no items")
			}
			wantCalls := 0
			if tt.lookupErr == nil {
				wantCalls = 1
			}
			if itemsCalls != wantCalls {
				t.Errorf("GetBasketItems called %d times, want %d", itemsCalls, wantCalls)
			}
		})
	}
}

func TestUpdateBacketItem(t *testing.T) {
	itemErr := errors.New("item lookup failed")
	basketErr := errors.New("basket lookup failed")
	updateErr := errors.New("update failed")
	tests := []struct {
		name       string
		foreign    bool
		itemErr    error
		basketErr  error
		updateErr  error
		wantUpdate bool
		wantErr    error
	}{
		{name: "own item", wantUpdate: true},
		{name: "foreign item", foreign: true, wantErr: ErrBasketItemNotFound},
		{name: "item not found", itemErr: gorm.ErrRecordNotFound, wantErr: gorm.ErrRecordNotFound},
		{name: "item lookup error", itemErr: itemErr, wantErr: itemErr},
		{name: "basket lookup error", basketErr: basketErr, wantErr: basketErr},
		{name: "update error", updateErr: updateErr, wantUpdate: true, wantErr: updateErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sessionID, itemID, basketID := uuid.New(), uuid.New(), uuid.New()
			item := models.BasketItem{ID: itemID, BasketID: basketID}
			if tt.foreign {
				item.BasketID = uuid.New()
			}
			params := map[string]any{"quantity": int64(20), "side": models.DealSideSell}
			itemCalls, basketCalls, updateCalls := 0, 0, 0
			store := &fakeStore{
				GetBasketItemByIDFunc: func(gotCtx context.Context, id uuid.UUID) (models.BasketItem, error) {
					itemCalls++
					if gotCtx != ctx || id != itemID {
						t.Error("item lookup received incorrect context or item ID")
					}
					if tt.itemErr != nil {
						return models.BasketItem{}, tt.itemErr
					}
					return item, nil
				},
			}
			if tt.itemErr == nil {
				store.GetBasketBySessionIDFunc = func(gotCtx context.Context, id uuid.UUID) (models.Basket, error) {
					basketCalls++
					if gotCtx != ctx || id != sessionID {
						t.Error("basket lookup received incorrect context or session ID")
					}
					if tt.basketErr != nil {
						return models.Basket{}, tt.basketErr
					}
					return models.Basket{ID: basketID, SessionID: sessionID}, nil
				}
			}
			if tt.wantUpdate {
				store.UpdateBasketItemFunc = func(gotCtx context.Context, id uuid.UUID, gotParams map[string]any) error {
					updateCalls++
					if gotCtx != ctx || id != itemID || !reflect.DeepEqual(gotParams, params) {
						t.Error("update received incorrect context, item ID or parameters")
					}
					return tt.updateErr
				}
			}

			err := NewService(store).UpdateBacketItem(ctx, sessionID, itemID, params)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			wantBasketCalls, wantUpdateCalls := 0, 0
			if tt.itemErr == nil {
				wantBasketCalls = 1
			}
			if tt.wantUpdate {
				wantUpdateCalls = 1
			}
			if itemCalls != 1 || basketCalls != wantBasketCalls || updateCalls != wantUpdateCalls {
				t.Errorf("calls: item=%d, basket=%d, update=%d; want 1, %d, %d", itemCalls, basketCalls, updateCalls, wantBasketCalls, wantUpdateCalls)
			}
		})
	}
}
