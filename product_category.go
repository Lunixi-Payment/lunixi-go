package lunixi

// Lunixi product category codes for BasketItem.ProductCategory. Payment
// methods with legal installment limits (e.g. shopping loan) and other
// platform features use them. The dictionary only grows by appending, so
// the field stays a plain string: a newer code can be sent before this SDK
// declares a constant for it. Unknown codes are rejected by the API (400).
const (
	ProductCategoryMobilePhone          = "MOBILE_PHONE"
	ProductCategoryTablet               = "TABLET"
	ProductCategoryComputer             = "COMPUTER"
	ProductCategoryTVAudio              = "TV_AUDIO"
	ProductCategoryWhiteGoods           = "WHITE_GOODS"
	ProductCategorySmallAppliance       = "SMALL_APPLIANCE"
	ProductCategoryCamera               = "CAMERA"
	ProductCategoryGameConsole          = "GAME_CONSOLE"
	ProductCategoryWearable             = "WEARABLE"
	ProductCategoryElectronicsAccessory = "ELECTRONICS_ACCESSORY"
	ProductCategoryElectronicsOther     = "ELECTRONICS_OTHER"
	ProductCategoryGold                 = "GOLD"
	ProductCategoryJewelry              = "JEWELRY"
	ProductCategoryWatch                = "WATCH"
	ProductCategoryApparel              = "APPAREL"
	ProductCategoryShoesBags            = "SHOES_BAGS"
	ProductCategoryCosmetics            = "COSMETICS"
	ProductCategoryFurniture            = "FURNITURE"
	ProductCategoryHomeLiving           = "HOME_LIVING"
	ProductCategoryGrocery              = "GROCERY"
	ProductCategoryBooksMedia           = "BOOKS_MEDIA"
	ProductCategoryToys                 = "TOYS"
	ProductCategorySportsOutdoor        = "SPORTS_OUTDOOR"
	ProductCategoryPetSupplies          = "PET_SUPPLIES"
	ProductCategoryAutoParts            = "AUTO_PARTS"
	ProductCategoryMusicalInstrument    = "MUSICAL_INSTRUMENT"
	ProductCategoryMicromobility        = "MICROMOBILITY"
	ProductCategoryVehicle              = "VEHICLE"
	ProductCategoryTransportTicket      = "TRANSPORT_TICKET"
	ProductCategoryEventTicket          = "EVENT_TICKET"
	ProductCategoryAccommodation        = "ACCOMMODATION"
	ProductCategoryTravelPackage        = "TRAVEL_PACKAGE"
	ProductCategoryHealthService        = "HEALTH_SERVICE"
	ProductCategoryEducation            = "EDUCATION"
	ProductCategoryShippingService      = "SHIPPING_SERVICE"
	ProductCategoryDigitalGoods         = "DIGITAL_GOODS"
	ProductCategoryGiftCard             = "GIFT_CARD"
	ProductCategorySubscription         = "SUBSCRIPTION"
	ProductCategoryServiceOther         = "SERVICE_OTHER"
	ProductCategoryOther                = "OTHER"
)

// ProductCategories returns the product category codes known to this SDK
// version, in dictionary order. The returned slice is a fresh copy.
func ProductCategories() []string {
	return []string{
		ProductCategoryMobilePhone,
		ProductCategoryTablet,
		ProductCategoryComputer,
		ProductCategoryTVAudio,
		ProductCategoryWhiteGoods,
		ProductCategorySmallAppliance,
		ProductCategoryCamera,
		ProductCategoryGameConsole,
		ProductCategoryWearable,
		ProductCategoryElectronicsAccessory,
		ProductCategoryElectronicsOther,
		ProductCategoryGold,
		ProductCategoryJewelry,
		ProductCategoryWatch,
		ProductCategoryApparel,
		ProductCategoryShoesBags,
		ProductCategoryCosmetics,
		ProductCategoryFurniture,
		ProductCategoryHomeLiving,
		ProductCategoryGrocery,
		ProductCategoryBooksMedia,
		ProductCategoryToys,
		ProductCategorySportsOutdoor,
		ProductCategoryPetSupplies,
		ProductCategoryAutoParts,
		ProductCategoryMusicalInstrument,
		ProductCategoryMicromobility,
		ProductCategoryVehicle,
		ProductCategoryTransportTicket,
		ProductCategoryEventTicket,
		ProductCategoryAccommodation,
		ProductCategoryTravelPackage,
		ProductCategoryHealthService,
		ProductCategoryEducation,
		ProductCategoryShippingService,
		ProductCategoryDigitalGoods,
		ProductCategoryGiftCard,
		ProductCategorySubscription,
		ProductCategoryServiceOther,
		ProductCategoryOther,
	}
}
