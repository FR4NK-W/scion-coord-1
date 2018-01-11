angular.module('scionApp')
    .controller('boxCtrl', ['$rootScope', '$scope', 'boxService', '$location', '$window', '$http',
        function ($rootScope, $scope, boxService, $location, $window, $http) {
            $scope.redirectIfNotAdmin = function () {
                if (!$rootScope.user["IsAdmin"]) {
                    $location.path('/user');
                }
            };

            $scope.boxEntryData = function () {
                boxService.boxPageData().then(
                    function (data) {
                        $rootScope.user = data["User"];
                        $scope.UpdateRequired = false;

                        $scope.redirectIfNotAdmin();
                        $scope.defaultRegistration = function () {
                            return {
                                UserEmail: "",
                                error: false
                            };
                        };
                        $scope.resetRegistrations = function () {
                            $scope.registrations = [$scope.defaultRegistration()];
                        };
                        $scope.resetRegistrations();
                    },
                    function (response) {
                        console.log(response);
                        if (response.status === 401 || response.status === 403) {
                            $location.path('/user');
                        }
                    });
            };

            $scope.boxEntryData();
            $scope.error = "";
            $scope.message = "";

            $scope.addChoice = function () {
                $scope.registrations.push($scope.defaultRegistration());
            };

            $scope.removeChoice = function () {
                $scope.registrations.pop();
            };

            $scope.dismissSuccess = function () {
                $scope.message = "";
            };

            $scope.dismissError = function () {
                $scope.error = "";
            };

            $scope.updateRegistrations = function (registrations) {
                boxService.updateRegistrations(registrations).then(
                    function (data) {
                        console.log(data);
                        if (data["boxes"].length === 0) {
                            $scope.error = "";
                            $scope.message = "All box registrations were successfull.";
                            $scope.resetRegistrations();
                        } else {
                            var failed_boxes = data["boxes"];
                            $scope.error = "Update failed for the following boxes: " + failed_boxes.join(", ");
                        }
                    }),
                    function (response) {
                        $scope.message = "Success";
                        console.log(response);
                    };
            };
        }
    ]);
