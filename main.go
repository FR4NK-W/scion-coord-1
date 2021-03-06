// Copyright 2016 ETH Zurich
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/astaxie/beego/orm"
	"github.com/didip/tollbooth"
	"github.com/didip/tollbooth/limiter"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/netsec-ethz/scion-coord/config"
	"github.com/netsec-ethz/scion-coord/controllers"
	"github.com/netsec-ethz/scion-coord/controllers/api"
	"github.com/netsec-ethz/scion-coord/controllers/middleware"
	"github.com/netsec-ethz/scion-coord/models"
	"github.com/netsec-ethz/scion-coord/utility"
)

// make sure that data about SCIONLab ASes in database is correct
// TODO (mlegner): remove deprecated servers?
func initializeSLS() error {
	sls, err := models.FindSCIONLabServer(config.SERVER_IA)
	vpnLastAssignedIPStart := utility.IPIncrement(config.SERVER_VPN_START_IP, -1)
	lastAssignedPortStart := config.SERVER_START_PORT - 1

	if err != nil {
		if err == orm.ErrNoRows { // Server does not exist
			newSLS := models.SCIONLabServer{
				IA:                config.SERVER_IA,
				IP:                config.SERVER_IP,
				LastAssignedPort:  lastAssignedPortStart,
				VPNIP:             config.SERVER_VPN_IP,
				VPNLastAssignedIP: vpnLastAssignedIPStart,
			}
			fmt.Println("Inserting SCIONLab AS configuration into database.")
			if err := newSLS.Insert(); err != nil {
				return fmt.Errorf("ERROR: Cannot insert SCIONLab AS configuration into database:"+
					" %v", err)
			}
		} else {
			return fmt.Errorf("ERROR: Cannot get SCIONLab AS configuration from database: %v", err)
		}
	} else { // Server exists and needs to be updated
		sls.IP = config.SERVER_IP
		sls.VPNIP = config.SERVER_VPN_IP
		if sls.LastAssignedPort < lastAssignedPortStart {
			sls.LastAssignedPort = lastAssignedPortStart - 1
		}
		if sls.VPNLastAssignedIP == "" || utility.IPCompare(sls.VPNLastAssignedIP,
			vpnLastAssignedIPStart) == -1 {
			sls.VPNLastAssignedIP = vpnLastAssignedIPStart
		}

		fmt.Printf("Updating SCIONLab AS configuration in database: %v", sls)
		if err := sls.Update(); err != nil {
			return fmt.Errorf("ERROR: Cannot update SCIONLab AS configuration in database: %v",
				err)
		}
	}

	return nil
}

// check if credential files exist and create necessary directories
func checkCredentials() bool {
	for _, f := range []string{api.TrcFile, api.CoreCertFile, api.CoreSigKey} {
		if _, err := os.Stat(f); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("ERROR: Please make sure that the necessary credential files exist.")
				fmt.Println("Consult the README.md for further details.")
			} else {
				fmt.Println("An error occurred when accessing " + f + ".")
			}
			return false
		}
	}
	os.MkdirAll(api.TempPath, os.ModePerm)
	os.MkdirAll(api.PackagePath, os.ModePerm)
	return true
}

func main() {
	// update database of SCIONLab ASes
	if err := initializeSLS(); err != nil {
		fmt.Printf("There was an error updating the server database: %v", err)
		return
	}

	if !checkCredentials() {
		return
	}

	// controllers
	registrationController := api.RegistrationController{}
	loginController := api.LoginController{}
	asController := api.ASController{}
	scionLabVMController := api.SCIONLabVMController{}

	// rate limitation
	resendLimit := tollbooth.NewLimiter(1, time.Minute*10,
		&limiter.ExpirableOptions{DefaultExpirationTTL: time.Hour})
	resendLimit.SetOnLimitReached(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Blocked %v from accessing '/api/resendLink' because of reached rate limit",
			w.Header().Get("X-Rate-Limit-Request-Remote-Addr"))
	})
	resendLimit.SetMessage("You can request an email every 10 minutes")

	// router
	router := mux.NewRouter()

	loggingChain := middleware.New(middleware.LoggingHandler)

	// public chain does not require authentication but serves back the XSRF Token
	xsrfChain := middleware.New(middleware.LoggingHandler, middleware.XSRFHandler)

	// Api chain goes through the authentication handler, which verifies either the session or the
	// account_id.secret combination
	apiChain := middleware.New(middleware.LoggingHandler, middleware.AuthHandler)

	// 404 on favicon requests
	router.Handle("/favicon.ico", http.HandlerFunc(http.NotFound))

	// index page
	router.Handle("/", xsrfChain.ThenFunc(controllers.Index))

	// ==========================================================
	// SCION Coord API

	// user registration
	router.Handle("/api/register", loggingChain.ThenFunc(
		registrationController.Register)).Methods("POST")
	router.Handle("/api/captchaSiteKey", loggingChain.ThenFunc(
		registrationController.LoadCaptchaSiteKey))

	// Resend verification email
	router.Handle("/api/resendLink", tollbooth.LimitHandler(resendLimit, loggingChain.ThenFunc(
		registrationController.ResendActivationLink))).Methods("POST")

	// user login
	router.Handle("/api/login", loggingChain.ThenFunc(loginController.Login))

	// user Logout
	router.Handle("/api/logout", loggingChain.ThenFunc(loginController.Logout))

	// user information
	router.Handle("/api/me", apiChain.ThenFunc(loginController.Me))

	//email validation
	router.Handle("/api/verifyEmail/{uuid}", loggingChain.ThenFunc(
		registrationController.VerifyEmail))

	// generates a SCIONLab VM
	// TODO(ercanucan): fix the authentication
	router.Handle("/api/as/generateVM", apiChain.ThenFunc(scionLabVMController.GenerateSCIONLabVM))
	router.Handle("/api/as/removeVM", apiChain.ThenFunc(scionLabVMController.RemoveSCIONLabVM))
	router.Handle("/api/as/downloads", apiChain.ThenFunc(scionLabVMController.ReturnTarball))
	router.Handle("/api/as/getSCIONLabVMASes/{account_id}/{secret}",
		apiChain.ThenFunc(scionLabVMController.GetSCIONLabVMASes))
	router.Handle("/api/as/confirmSCIONLabVMASes/{account_id}/{secret}",
		apiChain.ThenFunc(scionLabVMController.ConfirmSCIONLabVMASes))

	// ==========================================================
	// SCION Web API

	router.Handle("/api/as/exists/{as_id}/{account_id}/{secret}", apiChain.ThenFunc(
		asController.Exists))

	// ISD join request
	router.Handle("/api/as/uploadJoinRequest/{account_id}/{secret}", apiChain.ThenFunc(
		asController.UploadJoinRequest))
	router.Handle("/api/as/uploadJoinReply/{account_id}/{secret}", apiChain.ThenFunc(
		asController.UploadJoinReply))
	router.Handle("/api/as/pollJoinReply/{account_id}/{secret}", apiChain.ThenFunc(
		asController.PollJoinReply))

	// AS connection request
	router.Handle("/api/as/uploadConnRequest/{account_id}/{secret}", apiChain.ThenFunc(
		asController.UploadConnRequest))
	router.Handle("/api/as/uploadConnReply/{account_id}/{secret}", apiChain.ThenFunc(
		asController.UploadConnReply))

	// show all request TO this AS
	router.Handle("/api/as/pollEvents/{account_id}/{secret}", apiChain.ThenFunc(
		asController.PollEvents))

	// list the ASes the requesting AS can connect to
	router.Handle("/api/as/listASes/{account_id}/{secret}", apiChain.ThenFunc(
		asController.ListASes))

	// serve static files
	static := http.StripPrefix("/public/", http.FileServer(http.Dir("public")))
	router.PathPrefix("/public/").Handler(xsrfChain.Then(static))

	// listen to HTTP requests
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", config.HTTP_BIND_ADDRESS,
		config.HTTP_BIND_PORT), handlers.CompressHandler(router)))
}
