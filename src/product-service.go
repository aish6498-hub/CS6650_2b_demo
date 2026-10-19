package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

// Product : Gin's `required` tag treats a zero-value int as if it were omitted. A pointer lets `required` check for nil instead.
// `json` tags format outbound server responses, while `binding` tags validate inbound client requests.
type Product struct {
	ProductID    int32  `json:"product_id" binding:"required,min=1"`
	SKU          string `json:"sku" binding:"required,min=1,max=100"`
	Manufacturer string `json:"manufacturer" binding:"required,min=1,max=200"`
	CategoryID   int32  `json:"category_id" binding:"required,min=1"`
	Weight       *int32 `json:"weight" binding:"required,min=0"`
	SomeOtherID  int32  `json:"some_other_id" binding:"required,min=1"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// for testing 500 response
const reservedErrorID int32 = 999999

var (
	storeMu sync.RWMutex
	store   = map[int32]Product{}
)

// pre-seeded at startup
func seedStore() {
	w1, w2 := int32(1250), int32(500)
	store[1] = Product{ProductID: 1, SKU: "ABC-123-XYZ", Manufacturer: "Acme Corporation", CategoryID: 456, Weight: &w1, SomeOtherID: 789}
	store[2] = Product{ProductID: 2, SKU: "DEF-456-UVW", Manufacturer: "Globex Corporation", CategoryID: 457, Weight: &w2, SomeOtherID: 790}
}

func main() {
	seedStore()

	// gin.New() instead of gin.Default() to use our error recovery middleware
	// instead of Gin's default plain-text panic response.
	router := gin.New()
	router.Use(gin.Logger(), errorRecovery())

	router.GET("/products/:productId", handleGetProduct)
	router.POST("/products/:productId/details", handleAddProductDetails)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	router.Run(":" + port)
}

func errorRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. DEFER: Schedules this function to run at the very end.
		// Even if the code below crashes (panics), Go guarantees this block will execute.
		defer func() {
			// 2. RECOVER + IF: recover() intercepts and stops the server crash sequence.
			// If a panic happened, 'r' gets the error message (r != nil) and we enter the block.
			if r := recover(); r != nil {

				// 3. INSIDE THE IF BLOCK: The crash is caught. We can safely format
				// the error details and send a clean 500 JSON response to the user.
				c.JSON(http.StatusInternalServerError, ErrorResponse{
					Error:   "INTERNAL_ERROR",
					Message: "internal server error",
					Details: fmt.Sprint(r),
				})

				// 4. ABORT: Tells Gin to stop executing any remaining route handlers.
				c.Abort()
			}
		}()

		// 5. NEXT: Passes control to your actual business logic/endpoints.
		// If anything inside c.Next() crashes, execution instantly jumps up to line 5 (the defer).
		c.Next()
	}
}

func parseProductID(raw string) (int32, error) {
	id, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("productId must be a positive integer, got %q", raw)
	}
	// fmt.Errorf for predictable errors
	return int32(id), nil
}

func handleGetProduct(c *gin.Context) {
	id, err := parseProductID(c.Param("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "INVALID_INPUT", Message: "invalid productId", Details: err.Error()})
		return
	}
	if id == reservedErrorID {
		panic("simulated internal error, triggered via reserved productId for demo purposes")
	}

	storeMu.RLock()
	product, ok := store[id]
	storeMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "NOT_FOUND", Message: "product not found"})
		return
	}
	c.JSON(http.StatusOK, product)
}

func handleAddProductDetails(c *gin.Context) {
	id, err := parseProductID(c.Param("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "INVALID_INPUT", Message: "invalid productId", Details: err.Error()})
		return
	}

	// Declares a Product struct and uses c.ShouldBindJSON to map incoming JSON fields from the HTTP request body directly onto it.
	var body Product
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "INVALID_INPUT", Message: "request body failed validation", Details: err.Error()})
		return
	}
	// the path productId and the body's product_id both identify "which
	// product" - if they disagree, that's invalid input, not a lookup question
	if body.ProductID != id {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "INVALID_INPUT", Message: "path productId and body product_id must match"})
		return
	}
	if id == reservedErrorID {
		panic("simulated internal error, triggered via reserved productId for demo purposes")
	}

	storeMu.Lock()
	defer storeMu.Unlock() // POST uses defer because the locked region has a branch with an early return inside it
	if _, ok := store[id]; !ok {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "NOT_FOUND", Message: "product not found"})
		return
	}
	store[id] = body
	c.Status(http.StatusNoContent)
}
