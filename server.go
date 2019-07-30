package cashq

import (
	"encoding/json"
	"github.com/adonese/noebs/ebs_fields"
	"github.com/adonese/noebs/dashboard"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
	"gopkg.in/go-playground/validator.v9"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"net/http"
	"os"
	"strings"
)

var log = logrus.New()

func GetMainEngine() *gin.Engine {

	route := gin.Default()

	route.HandleMethodNotAllowed = true

	route.POST("/isAlive", IsAlive)

	//route.POST("/workingKey", WorkingKey)
	//route.POST("/cardTransfer", CardTransfer)
	//route.POST("/purchase", Purchase)
	//route.POST("/cashIn", CashIn)
	//route.POST("/cashOut", CashOut)
	//route.POST("/billInquiry", BillInquiry)
	//route.POST("/billPayment", BillPayment)
	//route.POST("/changePin", ChangePIN)
	//route.POST("/miniStatement", MiniStatement)
	//
	//route.POST("/balance", Balance)

	route.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": true})
	})

	dashboardGroup := route.Group("/dashboard")
	{
		dashboardGroup.GET("/get_tid", dashboard.TransactionByTid)
		dashboardGroup.GET("/get", dashboard.TransactionByTid)
		dashboardGroup.GET("/create", dashboard.MakeDummyTransaction)
		dashboardGroup.GET("/all", dashboard.GetAll)
		dashboardGroup.GET("/count", dashboard.TransactionsCount)
		dashboardGroup.GET("/settlement", dashboard.DailySettlement)
		dashboardGroup.GET("/metrics", gin.WrapH(promhttp.Handler()))
	}
	route.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	return route
}

func init() {
	// register the new validator
	binding.Validator = new(ebs_fields.DefaultValidator)
}

// @title noebs Example API
// @version 1.0
// @description This is a sample server celler server.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email adonese@soluspay/net
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host beta.soluspay.net
// @BasePath /api/
// @securityDefinitions.basic BasicAuth
// @in header
func main() {

	// logging and instrumentation
	file, err := os.OpenFile("logrus.log", os.O_CREATE|os.O_WRONLY, 0666)
	if err == nil {
		log.Out = file
	} else {
		log.Out = os.Stderr
		log.Info("Failed to log to file, using default stderr: %v", err)
	}
	log.Level = logrus.FatalLevel
	log.SetReportCaller(true) // get the method/function where the logging occured

	docs.SwaggerInfo.Title = "noebs Docs"

	if local := os.Getenv("EBS_LOCAL_DEV"); local != "" {
		UseMockServer = true
		log.WithFields(logrus.Fields{
			"ebs_local_flag": local,
		}).Info("User has opted to use the development server")
	} else {
		UseMockServer = false
		log.WithFields(logrus.Fields{
			"ebs_local_flag": local,
		}).Info("User has opted to not use the development server")
	}

	if env := os.Getenv("PORT"); env != "" {
		if !strings.HasPrefix(env, ":") {
			env += ":"
		} else {
			GetMainEngine().Run(env)
		}
	} else {
		GetMainEngine().Run(":8080")
	}
}

// IsAlive godoc
// @Summary Get all transactions made by a specific terminal ID
// @Description get accounts
// @Accept  json
// @Produce  json
// @Param workingKey body ebs_fields.IsAliveFields true "Working Key Request Fields"
// @Success 200 {object} main.SuccessfulResponse
// @Failure 400 {integer} 400
// @Failure 404 {integer} 404
// @Failure 500 {integer} 500
// @Router /workingKey [post]
func IsAlive(c *gin.Context) {

	url := EBSMerchantIP + IsAliveEndpoint // EBS simulator endpoint url goes here.

	db := database("sqlite3", "test.db")
	defer db.Close()

	var fields = ebs_fields.IsAliveFields{}

	// use bind to get free Form support rendering!
	// there is no practical need of using c.ShouldBindBodyWith;
	// Bind is more perfomant than ShouldBindBodyWith; the later copies the request body and reuse it
	// while Bind works directly on the responseBody stream.
	// More importantly, Bind smartly handles Forms rendering and validations; ShouldBindBodyWith forces you
	// into using only a *pre-specified* binding schema
	bindingErr := c.Bind(&fields)

	switch bindingErr := bindingErr.(type) {

	case validator.ValidationErrors:
		var details []ErrDetails

		for _, err := range bindingErr {

			details = append(details, ErrorToString(err))
		}

		payload := ErrorDetails{Details: details, Code: http.StatusBadRequest, Message: "Request fields validation error", Status: BadRequest}

		c.JSON(http.StatusBadRequest, payload)

	case nil:

		jsonBuffer, err := json.Marshal(fields)
		if err != nil {
			// there's an error in parsing the struct. Server error.
			er := ErrorDetails{Details: nil, Code: 400, Message: "Unable to parse the request", Status: ParsingError}
			c.AbortWithStatusJSON(400, er)
		}

		// the only part left is fixing EBS errors. Formalizing them per se.
		code, res, ebsErr := EBSHttpClient(url, jsonBuffer)
		log.Printf("response is: %d, %+v, %v", code, res, ebsErr)

		var successfulResponse SuccessfulResponse
		successfulResponse.EBSResponse = res

		transaction := dashboard.Transaction{
			GenericEBSResponseFields: res,
		}

		transaction.EBSServiceName = IsAliveTransaction

		// return a masked pan
		transaction.MaskPAN()

		// God please make it works.
		if err := db.Table("transactions").Create(&transaction).Error; err != nil {
			log.WithFields(logrus.Fields{
				"error":   err.Error(),
				"details": "Error in writing to database",
			}).Info("Problem in transaction table committing")
		}

		if ebsErr != nil {
			// convert ebs res code to int
			payload := ErrorDetails{Code: res.ResponseCode, Status: EBSError, Details: res, Message: EBSError}
			c.JSON(code, payload)
		} else {
			c.JSON(code, successfulResponse)
		}

	default:
		c.AbortWithStatusJSON(400, gin.H{"error": bindingErr.Error()})
	}
}