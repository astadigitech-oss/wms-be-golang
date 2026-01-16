package api

import (
	"liquid8/wms/http/controllers"
	"liquid8/wms/http/middleware"

	"github.com/gin-gonic/gin"
)

func RouteHandler(r *gin.Engine) {
	api := r.Group("/api") 

	// Route public
	// Authentication handler
	api.POST("/login", controllers.Login)
	api.GET("/checkLogin", controllers.CheckToken)

	// Route protected
	protected := api.Group("")
	protected.Use(middleware.AuthCheck())
	{
		// with rolecheck example
		// adminOnly := protected.Group("").Use(middleware.RoleCheck([]string{"Admin"}))
        // {
        //     adminOnly.POST("/generate", controllers.ProcessExcelHandler)
        //     adminOnly.POST("/generate/merge-headers", controllers.MapAndMergeHeaders)
        // }
		/* ==================== Dashboard ==================== */
		//StorageReport
		protected.GET("dashboard/storage-report", controllers.GetStorageReport) //DashboardController.go
		/* ==================== Inbound Routes ==================== */
		protected.POST("/generate", controllers.ProcessExcelHandler) // GenerateController.go
		protected.POST("/generate/merge-headers", controllers.MapAndMergeHeaders) //GenerateController.go
		//Bulking Product
		protected.POST("bulking/product/category", controllers.ImportBulkingCategory) //BulkingController.go
		// Manifest Inbound Routes
		protected.GET("/documents", controllers.IndexDocuments) // DocumentController.go
		protected.GET("/documents/:code/detail", controllers.DetailDocument) // DocumentController.go
		protected.GET("/documents/:code/search_old_product/:barcode", controllers.SearchProductOld) // DocumentController.go
		protected.GET("/documents/:code/user_scan_webs", controllers.GetUserScanWeb) // DocumentController.go
		protected.POST("/documents/custom-barcode", controllers.ChangeCustomBarcode) // DocumentController.go
		protected.DELETE("/documents/:code", controllers.DestroyDocument) // DocumentController.go
		protected.DELETE("/documents/product_old/:id", controllers.DestroyProductOld) // DocumentController.go
		protected.POST("/product-approve/:product_old_id", controllers.ProductApprove) // ProductController.go
		//Riwayat Check Routes
		protected.GET("/check-histories", controllers.CheckHistories); //DocumentController.go
		protected.GET("/check-histories/:history_id", controllers.DetailHistory); //DocumentController.go
		// Manual Inbound
		protected.POST("/products/manual", controllers.AddProductManual); //ProductController.go

		/* ==================== Stagging Routes ==================== */
		// staging
		protected.GET("/stagging-products", controllers.StaggingProduct) // ProductController.go
		protected.GET("/stagging/filter-products", controllers.StaggingFilterProduct) // ProductController.go
		protected.GET("/stagging-products/:product_id/detail", controllers.StaggingProductDetail) // ProductController.go
		protected.PUT("/products/:barcode/update", controllers.UpdateDataProduct) // ProductController.go
		protected.POST("/stagging/filter-products/:product_id", controllers.AddToFilterStaging) // ProductController.go
		protected.POST("/products/:barcode/to-damaged", controllers.ProductToDamaged) // ProductController.go
		protected.POST("/stagging-products", controllers.StaggingFilterApprove) // ProductController.go
		protected.DELETE("/stagging/filter-products/:product_id", controllers.DestroyFilterProduct) // ProductController.go
		// approvement stagging
		protected.GET("/stagging-approves", controllers.StaggingApprovement) // ProductController.go
		protected.POST("/stagging-approves", controllers.StaggingApprovesStore) // ProductController.go
		protected.DELETE("/stagging-approves/:product_id", controllers.DestroyStaggingApprove) // ProductController.go
		
		/* ==================== RACK ==================== */
		protected.GET("/racks", controllers.GetRacks) //RackController.go
		protected.GET("/racks/:rack_id/detail", controllers.RackDetail) //RackController.go
		protected.GET("/racks/list-product", controllers.ProductBySourceRack) //RackController.go
		protected.POST("/racks", controllers.AddRack) //RackController.go
		protected.POST("/racks/:rack_id/move-to-display", controllers.MoveRackToDisplay) //RackController.go
		protected.PUT("/racks/:rack_id", controllers.UpdateRack) //RackController.go
		protected.POST("/racks/:rack_id/add-product/:barcode", controllers.AddProductToRack) //RackController.go
		protected.DELETE("/racks/:rack_id/remove-product/:product_id", controllers.RemoveProductFromRack) //RackController.go
		protected.DELETE("/racks/:rack_id", controllers.DeleteRack) //RackController.go	
		// protected.PUT("/racks/:id", controllers.UpdateRack) //RackController.go

		/* ==================== INVENTORY ==================== */
		//Product
		protected.GET("/products/by-color", controllers.GetProductsByColor) // ProductController.go
		protected.GET("/products/by-category", controllers.GetProductsByCategory) // ProductController.go
		protected.GET("/products/status/display-expired", controllers.GetProductsStatusDisplayExpired) // ProductController.go
		protected.PUT("/products/:barcode/status-dump", controllers.ProductToDump) // ProductController.go
		protected.DELETE("/products/inventory/:id", controllers.DeleteProductInventory) // ProductController.go
		//category Setting
		protected.GET("/categories", controllers.Categories) //CategoryController.go
		protected.POST("/categories", controllers.AddCategory) //CategoryController.go
		protected.PUT("/categories/:id", controllers.UpdateCategory) //CategoryController.go
		protected.DELETE("/categories/:id", controllers.DeleteCategory) //CategoryController.go
		protected.GET("/color_tags", controllers.TagColors) //ColorTagController.go
		protected.POST("/color_tags", controllers.AddTagColor) //ColorTagController.go
		protected.PUT("/color_tags/:id", controllers.UpdateTagColor) //ColorTagController.go
		protected.DELETE("/color_tags/:id", controllers.DeleteTagColor) //ColorTagController.go
		//Moving Product -> bundle
		protected.GET("/bundles", controllers.GetBundles) //BundleController.go
		protected.GET("/bundle/filter-product", controllers.GetBundleFilterProduct) //BundleController.go
		//====================
		protected.GET("/bundles/:bundle_id/detail", controllers.GetBundleDetail) //BundleController.go 
		protected.POST("/bundle/:bundle_id/product-bundle/:product_id", controllers.AddProductBundle) //BundleController.go 
		protected.POST("/bundles", controllers.CreateBundleProduct) //BundleController.go
		protected.POST("/bundle/filter-product/:id", controllers.BundleAddFilterProduct) //BundleController.go
		protected.PUT("/bundles/:bundle_id", controllers.UpdateBundle) //BundleController.go
		protected.DELETE("/bundle/:bundle_id/product-bundle/:product_id", controllers.DeleteProductBundle) //BundleController.go
		protected.DELETE("/bundles/:bundle_id", controllers.Unbundle) //BundleController.go
		protected.DELETE("/bundle/filter-product/:id", controllers.BundleDeleteFilterProduct) //BundleController.go
		//====================
		//Moving Product -> repair
		protected.GET("/repair-bundles", controllers.GetRepairBundles) //BundleController.go
		protected.GET("/repair-bundle/filter-product", controllers.GetRepairFilterProduct) //BundleController.go
		protected.PUT("/repair-items/:item_id/dump", controllers.DumpProductRepair) //BundleController.go
		//Slow Moving Product -> promo
		protected.GET("/promos", controllers.GetPromos) // ProductController.go
		protected.POST("/promos", controllers.AddPromoProduct) // ProductController.go
		//Slow Moving Product - BKL
		protected.GET("/bkl-documents", controllers.ListBKLDocuments) //DocumentController.go
		protected.GET("/bkl-document/generate-code", controllers.GenerateBKLCode) //DocumentController.go
		protected.GET("/bkl-document/:id/detail", controllers.DetailBKL) //DocumentController.go
		protected.POST("/bkl-document", controllers.CreateBKL) //DocumentController.go
		protected.POST("/bkl-document/:id/to-edit", controllers.ToEditBKL) //DocumentController.go
		protected.PUT("/bkl-document/:id", controllers.UpdateBKL) //DocumentController.go
		//Stock Opname -> color
		protected.GET("/summary-so-colors", controllers.GetSummarySoColors) //SOController.go
		protected.GET("/summary-so-colors/:id", controllers.DetailSummarySoColor) //SOController.go
		protected.POST("/submit/so-color", controllers.SubmitSoColor) //SOController.go
		protected.POST("/start-so-color", controllers.StartSoColor) //SOController.go
		protected.POST("/stop-so-color", controllers.StopSoColor) //SOController.go
		//Stock Opname -> category
		protected.GET("/summary-so-categories", controllers.GetSummarySoCategories) //SOController.go
		protected.GET("/summary-so-categories/:id", controllers.DetailSummarySoCategory) //SOController.go
		protected.GET("/filter-so-category", controllers.FilterSoCategory) //SOController.go
		protected.GET("/search-so-category", controllers.SearchSoCategory) //SOController.go
		protected.POST("/update-check-so", controllers.UpdateCheck) //SOController.go
		protected.POST("/start-so-category", controllers.StartSoCategory) //SOController.go
		protected.POST("/stop-so-category", controllers.StopSoCategory) //SOController.go

		/* ==================== REPAIR STATION ==================== */
		//Migrate To Repair
		protected.GET("/migrate-repair-docs", controllers.ListMigrateRepairDocs) //DocumentController.go
		protected.GET("/migrate-repair-docs/:id", controllers.DetailMigrateRepairDocs) //DocumentController.go
		protected.GET("/migrate-products", controllers.ListMigrateProducts) //DocumentController.go
		protected.POST("/migrate-products/add", controllers.AddMigrateProduct) //DocumentController.go
		protected.PUT("/migrate-repair-docs/items/:item_id/update", controllers.MigrateProductUpdate) //DocumentController.go
		protected.PUT("/migrate-repair-docs/items/:item_id/to-display", controllers.MigrateProductToDisplay) //DocumentController.go
		protected.PUT("/migrate-repair-docs/items/:item_id/status-dump", controllers.MigrateProductToDump) //DocumentController.go
		//Abnormal
		protected.GET("/products/abnormal", controllers.GetProductAbnormal) //ProductController.go
		protected.PUT("/products/abnormal/:product_id/to-display", controllers.AbnormalToDisplay) //ProductController.go
		//Damaged
		protected.GET("/products/damaged", controllers.GetProductDamaged) //ProductController.go
		protected.PUT("/products/damaged/:product_id/to-display", controllers.DamagedToDisplay) //ProductController.go
		//Non
		protected.GET("/products/non", controllers.GetProductNon) //ProductController.go
		protected.PUT("/products/non/:product_id/to-display", controllers.NonToDisplay) //ProductController.go

		/* ==================== ACCOUNT ==================== */
		//Account Setting
		protected.GET("/users", controllers.GetUsers) //UserController.go
		protected.GET("/roles", controllers.GetRoles) //UserController.go
		protected.POST("/users", controllers.CreateUser) //UserController.go
		protected.PUT("/users/:id", controllers.UpdateUser) //UserController.go
		protected.DELETE("/users/:id", controllers.DeleteUser) //UserController.go

		/* ==================== OUTBOUND ==================== */
		//Buyer
		protected.GET("monthly-buyer", controllers.GetBuyerMonthlyPoints) //BuyerController.go
		protected.GET("summary-buyer", controllers.GetBuyerSummary) //BuyerController.go
		//QCD
		protected.GET("scraps", controllers.GetScrapDocuments) //DocumentController.go
		protected.GET("scraps/:scrap_id", controllers.DetailScrapDocuments) //DocumentController.go
		protected.GET("scrap/product-dumps", controllers.GetProductDumps) //DocumentController.go
		protected.GET("scrap/session", controllers.GetActiveSession) //DocumentController.go
		protected.POST("scraps/:scrap_id/scrap-all", controllers.AddAllProductToScrap) //DocumentController.go
		protected.POST("scraps/:scrap_id/product/:barcode", controllers.AddProductToScrap) //DocumentController.go
		protected.POST("scraps/:scrap_id/lock", controllers.LockScrapDocument) //DocumentController.go
		protected.POST("scraps/:scrap_id/finish", controllers.FinishScrapDocument) //DocumentController.go

		/* ==================== GENERALE ==================== */
		//Account Setting
		protected.GET("/product-price-colors", controllers.GetColorTagByPrice) //ColorTagController.go
	}
}