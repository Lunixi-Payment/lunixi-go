package lunixi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBasketItemProductCategoryIsOptional(t *testing.T) {
	item := BasketItem{ID: "sku-1", Name: "Example", ItemType: "PHYSICAL", Price: "10050"}
	plain, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "productCategory") {
		t.Fatalf("unset category was sent: %s", plain)
	}
	item.ProductCategory = ProductCategoryMobilePhone
	withCategory, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withCategory), `"productCategory":"MOBILE_PHONE"`) {
		t.Fatalf("category not serialized as productCategory: %s", withCategory)
	}
}

func TestProductCategoriesFollowDictionaryOrder(t *testing.T) {
	codes := ProductCategories()
	if len(codes) != 40 || codes[0] != "MOBILE_PHONE" || codes[len(codes)-1] != "OTHER" {
		t.Fatalf("unexpected dictionary: %v", codes)
	}
	codes[0] = "CHANGED"
	if ProductCategories()[0] != ProductCategoryMobilePhone {
		t.Fatal("ProductCategories exposed shared state")
	}
}
