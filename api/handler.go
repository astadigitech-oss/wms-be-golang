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

		/* ==================== Inbound Routes ==================== */
		protected.POST("/generate", controllers.ProcessExcelHandler) // GenerateController.go
		protected.POST("/generate/merge-headers", controllers.MapAndMergeHeaders) //GenerateController.go

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
		protected.PUT("/stagging-products/:product_id", controllers.StaggingProductUpdate) // ProductController.go
		protected.POST("/stagging/filter-products/:product_id", controllers.AddToFilterStaging) // ProductController.go
		protected.POST("/stagging/move-to-lpr/:product_id", controllers.StaggingMoveToLPR) // ProductController.go
		protected.POST("/stagging-products", controllers.StaggingFilterApprove) // ProductController.go
		protected.DELETE("/stagging/filter-products/:product_id", controllers.DestroyFilterProduct) // ProductController.go
		// approvement stagging
		protected.GET("/stagging-approves", controllers.StaggingApprovement) // ProductController.go
		protected.POST("/stagging-approves", controllers.StaggingApprovesStore) // ProductController.go
		protected.DELETE("/stagging-approves/:product_id", controllers.DestroyStaggingApprove) // ProductController.go
		
		/* ==================== INVENTORY ==================== */
		//Product
		protected.GET("/products/by-color", controllers.GetProductsByColor) // ProductController.go
		protected.GET("/products/by-category", controllers.GetProductsByCategory) // ProductController.go
		protected.GET("/products/status/display-expired", controllers.GetProductsStatusDisplayExpired) // ProductController.go
		protected.PUT("/products/:id/status-dump", controllers.UpdateProductStatus) // ProductController.go
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
		
		protected.GET("/bundles/:bundle_id/detail", controllers.GetBundleDetail) //BundleController.go 
		protected.POST("/bundle/:bundle_id/product-bundle/:product_id", controllers.AddProductBundle) //BundleController.go 
		protected.POST("/bundles", controllers.CreateBundleProduct) //BundleController.go
		protected.POST("/bundle/filter-product/:id", controllers.BundleAddFilterProduct) //BundleController.go
		protected.PUT("/bundles/:bundle_id", controllers.UpdateBundle) //BundleController.go
		protected.DELETE("/bundle/:bundle_id/product-bundle/:product_id", controllers.DeleteProductBundle) //BundleController.go
		protected.DELETE("/bundles/:bundle_id", controllers.Unbundle) //BundleController.go
		protected.DELETE("/bundle/filter-product/:id", controllers.BundleDeleteFilterProduct) //BundleController.go
		
		//Moving Product -> repair
		protected.GET("/repair-bundles", controllers.GetRepairBundles) //BundleController.go
		protected.GET("/repair-bundle/filter-product", controllers.GetRepairFilterProduct) //BundleController.go
		protected.PUT("/repair-items/:item_id/dump", controllers.DumpProductRepair) //BundleController.go
	}
}