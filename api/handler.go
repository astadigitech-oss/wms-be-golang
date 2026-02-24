package api

import (
	"liquid8/wms/http/controllers"
	"liquid8/wms/http/middleware"

	"github.com/gin-gonic/gin"
)

func roleGroup(g *gin.RouterGroup, roles []string, fn func(rg *gin.RouterGroup)) {
    rg := g.Group("")
    rg.Use(middleware.RoleCheck(roles))
    fn(rg)
}



func RouteHandler(r *gin.Engine) {
	api := r.Group("/api") 

	// Route public
	// Authentication handler
	api.POST("/login", controllers.Login)
	api.GET("/checkLogin", controllers.CheckToken)

	// // with rolecheck example
	// adminOnly := protected.Group("").Use(middleware.RoleCheck([]string{"Admin"}))
	// {
	// 	adminOnly.POST("/generate", controllers.ProcessExcelHandler)
	// 	adminOnly.POST("/generate/merge-headers", controllers.MapAndMergeHeaders)
	// }

	// Route protected
	protected := api.Group("")
	protected.Use(middleware.AuthCheck())
	{
		/* ==================== Dashboard ==================== */
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Admin Kasir"}, func(rg *gin.RouterGroup) {
			//storage-report
			rg.GET("dashboard/storage-report", controllers.GetStorageReport) //DashboardController.go
			rg.GET("dashboard/storage-report/export", controllers.ExportStorageReport) //DashboardController.go
			rg.GET("dashboard/archive-storage-report/export", controllers.ExportArchiveStorageReport) //DashboardController.go
			//general-sales
			rg.GET("dashboard/general-sales", controllers.GetGeneralSales) //DashboardController.go
			rg.GET("dashboard/monthly-analytic-sales/export", controllers.ExportMonthlyAnalyticSales) //DashboardController.go
			rg.GET("dashboard/yearly-analytic-sales/export", controllers.ExportYearlyAnalyticSales) //DashboardController.go
			//analytic-sales
			rg.GET("dashboard/monthly-analytic-sales", controllers.GetMonthlyAnalyticSale) //DashboardController.go
			rg.GET("dashboard/yearly-analytic-sales", controllers.GetYearlyAnalyticSale) //DashboardController.go
		})
		//summary report all role
		protected.GET("dashboard/summary-begin-balance", controllers.SummaryBeginBalance) //SummaryController.go
		protected.GET("dashboard/summary-ending-balance", controllers.SummaryEndingBalance) //SummaryController.go
		protected.GET("dashboard/list-summary-both", controllers.ListSummaryBoth) //SummaryController.go
		/* ==================== Inbound Routes ==================== */
		roleGroup(protected, []string{"Spv", "Team leader"}, func(rg *gin.RouterGroup) {
			rg.POST("/generate", controllers.ProcessExcelHandler) // GenerateController.go
			rg.POST("/generate/merge-headers", controllers.MapAndMergeHeaders) //GenerateController.go
			//Bulking Product
			rg.POST("/bulking/product/category", controllers.ImportBulkingCategory) //BulkingController.go
			rg.POST("/bulking/product/color", controllers.ImportBulkingColor) //BulkingController.go
		})
		//SKU Route
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Admin Kasir", "Crew", "Reparasi", "Kasir Leader", "Developer"}, func(rg *gin.RouterGroup) {
			//inbound sku
			rg.POST("/generate/sku", controllers.ImportExcelSkuHandler) // SkuController.go
			rg.POST("/generate/sku/merge-headers", controllers.MapAndMergeHeadersSku) // SkuController.go
			//manifest inbound sku
			rg.GET("sku-documents", controllers.SkuDocuments) //SkuController.go
			rg.GET("sku-documents/:code/detail", controllers.DetailSkuDocument) //SkuController.go
			rg.GET("sku-documents/:code/export", controllers.ExportSkuDocument) //SkuController.go
			rg.POST("sku-documents/:code/submit", controllers.SubmitSku) //SkuController.go
			rg.POST("sku-documents/custom-barcode", controllers.SkuCustomBarcode) //SkuController.go
			rg.PUT("sku-product-olds/:product_id", controllers.UpdateSkuProductOld) //SkuController.go
			rg.DELETE("sku-documents/:code", controllers.DestroySkuDocument) //SkuController.go
			//inventory - by sku
			rg.GET("sku-documents/:code/products", controllers.SkuProducts) //SkuController.go
			rg.GET("sku-documents/:code/histories", controllers.GetHistoryBundling) //SkuController.go
			rg.GET("sku-products/:sku_product_id/detail", controllers.DetailSkuProduct) //SkuController.go
			rg.POST("sku-products/:sku_product_id/damages", controllers.SkuStoreDamaged) //SkuController.go
			rg.POST("sku-products/check-type", controllers.CheckTypeBundleSku) //SkuController.go
			rg.POST("sku-products/:sku_product_id/bundles", controllers.SkuStoreBundle) //SkuController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Crew"}, func(rg *gin.RouterGroup) {
			// Manifest Inbound
			rg.GET("/documents", controllers.IndexDocuments) // DocumentController.go
			rg.GET("/documents/:code/detail", controllers.DetailDocument) // DocumentController.go
			rg.GET("/documents/:code/search_old_product/:barcode", controllers.SearchProductOld) // DocumentController.go
			rg.GET("/documents/:code/user_scan_webs", controllers.GetUserScanWeb) // DocumentController.go
			rg.POST("/documents/custom-barcode", controllers.ChangeCustomBarcode) // DocumentController.go
			rg.DELETE("/documents/:code", controllers.DestroyDocument) // DocumentController.go
			rg.DELETE("/documents/product_old/:id", controllers.DestroyProductOld) // DocumentController.go
			rg.POST("/product-approve/:product_old_id", controllers.ProductApprove) // ProductController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader"}, func(rg *gin.RouterGroup) {
			// Manual Inbound
			rg.POST("/products/manual", controllers.AddProductManual) //ProductController.go
			//Riwayat Check Routes
			rg.GET("/check-histories", controllers.CheckHistories) //DocumentController.go
			rg.GET("/check-histories/:history_id", controllers.DetailHistory) //DocumentController.go
		})

		/* ==================== Stagging Routes ==================== */
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Kasir leader"}, func(rg *gin.RouterGroup) {
			// staging
			rg.GET("/stagging-products", controllers.StaggingProduct) // ProductController.go
			rg.GET("/stagging-products/export", controllers.ExportStagingProduct) // ProductController.go
			rg.GET("/stagging/filter-products", controllers.StaggingFilterProduct) // ProductController.go
			rg.GET("/stagging-products/:product_id/detail", controllers.StaggingProductDetail) // ProductController.go
			rg.PUT("/products/:barcode/update", controllers.UpdateDataProduct) // ProductController.go
			rg.POST("/stagging/filter-products/:product_id", controllers.AddToFilterStaging) // ProductController.go
			rg.POST("/stagging-products", controllers.StaggingFilterApprove) // ProductController.go
			rg.DELETE("/stagging/filter-products/:product_id", controllers.DestroyFilterProduct) // ProductController.go
			// approvement stagging
			rg.GET("/stagging-approves", controllers.StaggingApprovement) // ProductController.go
			rg.POST("/stagging-approves", controllers.StaggingApprovesStore) // ProductController.go
			rg.DELETE("/stagging-approves/:product_id", controllers.DestroyStaggingApprove) // ProductController.go
			
			/* ==================== RACK ==================== */
			rg.GET("/racks", controllers.GetRacks) //RackController.go
			rg.GET("/racks/:rack_id/detail", controllers.RackDetail) //RackController.go
			rg.GET("/racks/list-product", controllers.ProductBySourceRack) //RackController.go
			rg.POST("/racks", controllers.AddRack) //RackController.go
			rg.POST("/racks/:rack_id/move-to-display", controllers.MoveRackToDisplay) //RackController.go
			rg.PUT("/racks/:rack_id", controllers.UpdateRack) //RackController.go
			rg.POST("/racks/:rack_id/add-product/:barcode", controllers.AddProductToRack) //RackController.go
			rg.DELETE("/racks/:rack_id/remove-product/:product_id", controllers.RemoveProductFromRack) //RackController.go
			rg.DELETE("/racks/:rack_id", controllers.DeleteRack) //RackController.go	
			// protected.PUT("/racks/:id", controllers.UpdateRack) //RackController.go
		})

		/* ==================== INVENTORY ==================== */
		//Product
		roleGroup(protected, []string{"Admin", "Spv", "Team leader"}, func(rg *gin.RouterGroup) {
			rg.GET("/products/by-color", controllers.GetProductsByColor) // ProductController.go
		})
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Kasir leader"}, func(rg *gin.RouterGroup) {
			rg.GET("/products/by-category", controllers.GetProductsByCategory) // ProductController.go
			rg.POST("/products/:barcode/to-damaged", controllers.ProductToDamaged) // ProductController.go
		})	
		
		protected.GET("/products/status/display-expired", controllers.GetProductsStatusDisplayExpired) // ProductController.go
		protected.GET("/products/:product_id/detail", controllers.GetDetailProduct) // ProductController.go
		protected.PUT("/products/:barcode/status-dump", controllers.ProductToDump) // ProductController.go
		protected.DELETE("/products/inventory/:id", controllers.DeleteProductInventory) // ProductController.go

		roleGroup(protected, []string{"Admin", "Spv"}, func(rg *gin.RouterGroup) {
			//category Setting
			rg.GET("/categories", controllers.Categories) //CategoryController.go
			rg.GET("/categories/export", controllers.ExportCategory) //CategoryController.go
			rg.POST("/categories", controllers.AddCategory) //CategoryController.go
			rg.PUT("/categories/:id", controllers.UpdateCategory) //CategoryController.go
			rg.DELETE("/categories/:id", controllers.DeleteCategory) //CategoryController.go
			rg.GET("/color_tags", controllers.TagColors) //ColorTagController.go
			rg.POST("/color_tags", controllers.AddTagColor) //ColorTagController.go
			rg.PUT("/color_tags/:id", controllers.UpdateTagColor) //ColorTagController.go
			rg.DELETE("/color_tags/:id", controllers.DeleteTagColor) //ColorTagController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Crew"}, func(rg *gin.RouterGroup) {
			//Moving Product -> bundle
			rg.GET("/bundles", controllers.GetBundles) //BundleController.go
			rg.GET("/bundle/product-type-colors", controllers.GetProductTypeColor) //BundleController.go
			rg.GET("/bundle/filter-product", controllers.GetBundleFilterProduct) //BundleController.go
			rg.GET("/bundles/:bundle_id/detail", controllers.GetBundleDetail) //BundleController.go 
			rg.POST("/bundle/:bundle_id/product-bundle/:product_id", controllers.AddProductBundle) //BundleController.go 
			rg.POST("/bundles", controllers.CreateBundleProduct) //BundleController.go
			rg.POST("/bundle/filter-product/:id", controllers.BundleAddFilterProduct) //BundleController.go
			rg.PUT("/bundles/:bundle_id", controllers.UpdateBundle) //BundleController.go
			rg.DELETE("/bundle/items/:item_id", controllers.DeleteProductBundle) //BundleController.go
			rg.DELETE("/bundles/:bundle_id", controllers.Unbundle) //BundleController.go
			rg.DELETE("/bundle/filter-product/:id", controllers.BundleDeleteFilterProduct) //BundleController.go
		})
		//====================
		//Moving Product -> repair
		// protected.GET("/repair-bundles", controllers.GetRepairBundles) //BundleController.go
		// protected.GET("/repair-bundle/filter-product", controllers.GetRepairFilterProduct) //BundleController.go
		// protected.PUT("/repair-items/:item_id/update", controllers.UpdateRepairProduct) //BundleController.go
		// protected.PUT("/repair-items/:item_id/to-display", controllers.ProductRepairToDisplay) //BundleController.go
		// protected.PUT("/repair-items/:item_id/dump", controllers.DumpProductRepair) //BundleController.go
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Kasir leader"}, func(rg *gin.RouterGroup) {
			//Slow Moving Product -> promo
			rg.GET("/promos", controllers.GetPromos) // ProductController.go
			rg.POST("/promos", controllers.AddPromoProduct) // ProductController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Crew"}, func(rg *gin.RouterGroup) {
			//Slow Moving Product - BKL
			rg.GET("/bkl-documents", controllers.ListBKLDocuments) //DocumentController.go
			rg.GET("/bkl-document/generate-code", controllers.GenerateBKLCode) //DocumentController.go
			rg.GET("/bkl-document/:id/detail", controllers.DetailBKL) //DocumentController.go
			rg.POST("/bkl-document", controllers.CreateBKL) //DocumentController.go
			rg.POST("/bkl-document/:id/to-edit", controllers.ToEditBKL) //DocumentController.go
			rg.PUT("/bkl-document/:id", controllers.UpdateBKL) //DocumentController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader"}, func(rg *gin.RouterGroup) {
			//Stock Opname -> color
			rg.GET("/summary-so-colors", controllers.GetSummarySoColors) //SOController.go
			rg.GET("/summary-so-colors/:id", controllers.DetailSummarySoColor) //SOController.go
			rg.POST("/submit/so-color", controllers.SubmitSoColor) //SOController.go
			rg.POST("/start-so-color", controllers.StartSoColor) //SOController.go
			rg.POST("/stop-so-color", controllers.StopSoColor) //SOController.go
		})

		roleGroup(protected, []string{"Admin", "Spv"}, func(rg *gin.RouterGroup) {
			//Stock Opname -> category
			rg.GET("/summary-so-categories", controllers.GetSummarySoCategories) //SOController.go
			rg.GET("/summary-so-categories/:id", controllers.DetailSummarySoCategory) //SOController.go
			rg.GET("/filter-so-category", controllers.FilterSoCategory) //SOController.go
			rg.GET("/search-so-category", controllers.SearchSoCategory) //SOController.go
			rg.POST("/update-check-so", controllers.UpdateCheck) //SOController.go
			rg.POST("/start-so-category", controllers.StartSoCategory) //SOController.go
			rg.POST("/stop-so-category", controllers.StopSoCategory) //SOController.go
		})

		// ========================================================================================================
		// REPAIR STATION
		// ========================================================================================================
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Kasir leader", "Reparasi", "Admin Kasir"}, func(rg *gin.RouterGroup) {
			//Migrate To Repair
			rg.GET("/migrate-repair-docs", controllers.ListMigrateRepairDocs) //DocumentController.go
			rg.GET("/migrate-repair-docs/:id", controllers.DetailMigrateRepairDocs) //DocumentController.go
			rg.GET("/migrate-repair-index", controllers.GetMigrateRepairIndex) //DocumentController.go
			rg.GET("/migrate-products", controllers.ListMigrateProducts) //DocumentController.go
			rg.POST("/migrate-repair-docs/finish", controllers.MigrateRepairDone) //DocumentController.go
			rg.POST("/migrate-products/add", controllers.AddMigrateProduct) //DocumentController.go
			rg.PUT("/migrate-repair-docs/items/:item_id/update", controllers.MigrateProductUpdate) //DocumentController.go
			rg.PUT("/migrate-repair-docs/items/:item_id/to-display", controllers.MigrateProductToDisplay) //DocumentController.go
			rg.PUT("/migrate-repair-docs/items/:item_id/status-dump", controllers.MigrateProductToDump) //DocumentController.go
			rg.DELETE("/migrate-repair-docs/items/:item_id", controllers.DeleteMigrateRepairItem) //DocumentController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Reparasi"}, func(rg *gin.RouterGroup) {
			//Abnormal
			rg.GET("/products/abnormal", controllers.GetProductAbnormal) //ProductController.go
			rg.GET("/products/abnormal/export", controllers.ExportAbnormalProduct) //ProductController.go
			rg.PUT("/products/abnormal/:product_id/to-display", controllers.AbnormalToDisplay) //ProductController.go
			//Damaged
			rg.GET("/products/damaged", controllers.GetProductDamageds) //ProductController.go
			//Non
			rg.GET("/products/non", controllers.GetProductNon) //ProductController.go
			rg.PUT("/products/non/:product_id/to-display", controllers.NonToDisplay) //ProductController.go

			//RepairDocument (route for document damaged & non)
			rg.GET("/repair/:type/documents/", controllers.GetRepairDocuments) //ProductController.go
			rg.GET("/repair/:type/documents/:doc_id/export", controllers.ExportRepairDocument) //ProductController.go
			rg.GET("/repair/:type/documents/export", controllers.ExportAllProductRepairByType) //ProductController.go
			rg.GET("repair/:type/documents/:doc_id", controllers.DetailRepairDocuments) //ProductController.go
			rg.GET("repair/:type/document/active-session", controllers.GetActiveSessionRepairDoc) //ProductController.go
			rg.POST("repair/document/repair-all", controllers.AddAllProductToRepairDocument) //ProductController.go
			rg.POST("repair/document/product", controllers.AddProductToRepairDocument) //ProductController.go
			rg.POST("repair/:type/document/:doc_id/lock", controllers.LockRepairDocument) //ProductController.go
			rg.POST("repair/:type/document/:doc_id/finish", controllers.FinishRepairDocument) //ProductController.go
			rg.DELETE("repair/document/product", controllers.RemoveProductRepairDocument) //ProductController.go
		})

		/* ==================== OUTBOUND ==================== */
		roleGroup(protected, []string{"Admin", "Spv", "Team leader"}, func(rg *gin.RouterGroup) {
			//Migrate Color
			rg.GET("migrate-color/documents", controllers.GetMigrateDocuments)  //MigrateColorController.go
			rg.GET("migrate-color/documents/:doc_id", controllers.DetailMigrateDocument)  //MigrateColorController.go
			rg.GET("display-active-document", controllers.GetActiveMigrateDocument)  //MigrateColorController.go
			rg.GET("color-destinations", controllers.GetColorDestination)  //MigrateColorController.go
			rg.GET("migrate-color/destinations", controllers.GetMigrateDestinations)  //MigrateColorController.go
			rg.POST("migrates", controllers.StoreMigrateColor)  //MigrateColorController.go
			rg.POST("migrate-color/documents/finish", controllers.MigrateDocumentFinish)  //MigrateColorController.go
			rg.POST("migrate-color/destinations", controllers.StoreMigrateDestination)  //MigrateColorController.go
			rg.PUT("migrate-color/destinations/:destination_id", controllers.UpdateMigrateDestination)  //MigrateColorController.go
			rg.DELETE("migrates/:migrate_id", controllers.DestroyMigrateColor)  //MigrateColorController.go
			rg.DELETE("migrate-color/destinations/:destination_id", controllers.DestroyMigrateDestination)  //MigrateColorController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Admin Kasir", "Kasir leader"}, func(rg *gin.RouterGroup) {
			//sale
			rg.GET("sale-documents", controllers.GetSaleDocuments) //SaleController.go
			rg.GET("sale-documents/:sale_doc_id", controllers.DetailSaleDocument) //SaleController.go
			rg.GET("sale-documents/invoice-sale/:sale_doc_id", controllers.ExportInvoiceSale) //SaleController.go
			rg.GET("sales", controllers.SaleIndex) //SaleController.go
			rg.GET("sale/products", controllers.GetProductsForSale) //ProductController.go
			rg.POST("sale-documents/add-product", controllers.AddProductToSaleDocument) //SaleController.go
			rg.POST("sale/products/add", controllers.StoreProductToSale) //SaleController.go
			rg.POST("sales/finish", controllers.SaleFinish) //SaleController.go
			rg.PUT("sale-documents/:sale_doc_id", controllers.UpdateSaleDocument) //SaleController.go
			rg.PUT("sales/:sale_id/update-price", controllers.UpdatePriceSale) //SaleController.go
			rg.DELETE("sales/:sale_id", controllers.DestroySale) //SaleController.go
			rg.GET("ppn", controllers.GetPPN) //PPNController.go
			rg.POST("ppn", controllers.StorePPN) //PPNController.go
			rg.PUT("ppn/:ppn_id", controllers.UpdatePPN) //PPNController.go
			rg.DELETE("ppn/:ppn_id", controllers.DeletePPN) //PPNController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Crew"}, func(rg *gin.RouterGroup) {
			//B2B
			rg.GET("bulky-documents", controllers.GetBulkyDocuments) //BulkyController.go
			rg.GET("bulky-documents/:doc_id/bags", controllers.BagByUser) //BulkyController.go
			rg.GET("bulky-documents/bags/:bag_id", controllers.ShowBagProductDetail) //BulkyController.go
			rg.GET("bulky-documents/:doc_id/detail", controllers.DetailBulkyDocument) //BulkyController.go
			rg.POST("bulky-documents", controllers.CreateBulkyDocument) //BulkyController.go
			rg.POST("bulky-documents/:doc_id/bags", controllers.StoreBagBulkyDocument) //BulkyController.go
			rg.POST("bulky-documents/:doc_id/export", controllers.ExportBulkyDocument) //BulkyController.go
			rg.POST("bulky-sales/product", controllers.StoreBulkySale) //BulkyController.go
			rg.POST("bulky-sales/product/import", controllers.ImportFileBulkySale) //BulkyController.go
			rg.PUT("bulky-documents/:bulky_doc_id", controllers.UpdateBulkyDocument) //BulkyController.go
			rg.PUT("bulky-documents/:bulky_doc_id/finish", controllers.BulkyDocumentFinish) //BulkyController.go
			rg.DELETE("bulky-documents/bags/:bag_id", controllers.DestroyBagBulkyDocument) //BulkyController.go
			rg.DELETE("bulky-sales/product/:bulky_sale_id", controllers.DeleteBulkySale) //BulkyController.go
		})
		roleGroup(protected, []string{"Admin", "Spv", "Admin Kasir", "Kasir leader"}, func(rg *gin.RouterGroup) {
			//Buyer
			rg.GET("buyers", controllers.GetBuyers) //BuyerController.go
			rg.GET("buyers/:buyer_id", controllers.DetailBuyer) //BuyerController.go
			rg.GET("monthly-buyer", controllers.GetBuyerMonthlyPoints) //BuyerController.go
			rg.GET("summary-buyer", controllers.GetBuyerSummary) //BuyerController.go
			rg.POST("buyers", controllers.StoreBuyer) //BuyerController.go
			rg.PUT("buyers/:buyer_id", controllers.UpdateBuyer) //BuyerController.go
		})

		roleGroup(protected, []string{"Admin", "Spv", "Reparasi"}, func(rg *gin.RouterGroup) {
			//QCD
			rg.GET("scraps", controllers.GetScrapDocuments) //DocumentController.go
			rg.GET("scraps/documents/:doc_id/export", controllers.ExportScrapQcdDocument) //DocumentController.go
			rg.GET("scraps/summary/export", controllers.ExportSummaryScrapQcd) //DocumentController.go
			rg.GET("scraps/:scrap_id", controllers.DetailScrapDocuments) //DocumentController.go
			rg.GET("scrap/product-dumps", controllers.GetProductDumps) //DocumentController.go
			rg.GET("scrap/session", controllers.GetActiveSession) //DocumentController.go
			rg.POST("scraps/:scrap_id/scrap-all", controllers.AddAllProductToScrap) //DocumentController.go
			rg.POST("scraps/:scrap_id/product/:barcode", controllers.AddProductToScrap) //DocumentController.go
			rg.POST("scraps/:scrap_id/lock", controllers.LockScrapDocument) //DocumentController.go
			rg.POST("scraps/:scrap_id/finish", controllers.FinishScrapDocument) //DocumentController.go
		})
		//Pallet Bulky

		/* ==================== ACCOUNT ==================== */
		roleGroup(protected, []string{"Admin", "Spv"}, func(rg *gin.RouterGroup) {
			//Account Setting
			rg.GET("/users", controllers.GetUsers) //UserController.go
			rg.GET("/roles", controllers.GetRoles) //UserController.go
			rg.POST("/users", controllers.CreateUser) //UserController.go
			rg.PUT("/users/:id", controllers.UpdateUser) //UserController.go
			rg.DELETE("/users/:id", controllers.DeleteUser) //UserController.go
		})

		/* ==================== GENERALE ==================== */
		protected.GET("/product-price-colors", controllers.GetColorTagByPrice) //ColorTagController.go

		//notification
		protected.GET("/notif-widget", controllers.NotifWidget) //NotificationController.go
		protected.GET("/notifications", controllers.GetNotifications) //NotificationController.go
		protected.GET("/notifications/:notif_id/:status", controllers.GetApproveSPV) //NotificationController.go
		protected.GET("/approve-edit/:approve_id", controllers.ApproveEdit) //NotificationController.go
		protected.GET("/reject-edit/:approve_id", controllers.RejectEdit) //NotificationController.go

		// ========================================================================================================
		// SO Product
		// ========================================================================================================
		roleGroup(protected, []string{"Admin", "Spv", "Team leader", "Admin Kasir", "Crew", "Reparasi", "Kasir leader", "Developer"}, func(rg *gin.RouterGroup) {
			rg.POST("/racks/so", controllers.SoRackByBarcode) // SOController.go
			rg.POST("/racks/:rack_id/so", controllers.SoRackByID) // SOController.go
			rg.POST("/racks/so-staging-display", controllers.SoScanInDisplayRack) // SOController.go
			rg.POST("/display-products/:barcode/so", controllers.SoProductDisplay) // SOController.go
			rg.POST("/staging-products/:barcode/so", controllers.SoProductStaging) // SOController.go
			rg.POST("/abnormal-products/:barcode/so", controllers.SoProductAbnormal) //SOController.go
			rg.POST("/migrate-products/:barcode/so", controllers.SoProductMigrateRepair) //SOController.go
			rg.POST("/damaged-products/:barcode/so", controllers.SoProductDamaged) //SOController.go
			rg.POST("/non-products/:barcode/so", controllers.SoProductNon) //SOController.go
			rg.POST("/b2b-documents/so", controllers.SoB2BDocument) //SOController.go
		})
		
	}
}