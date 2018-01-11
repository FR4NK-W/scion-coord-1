angular.module('scionApp')
    .factory('boxService', ["$http", "$q", function ($http, $q) {
        return {
            boxPageData: function () {
                return $http.get('/api/adminPageData').then(function (response) {
                    console.log(response);
                    return response.data;
                });
            },
            updateRegistrations: function (registrations) {
                console.log("Sent request: ");
                console.log(angular.toJson(registrations));
                return $http.post('/api/updateRegistrations', angular.toJson(registrations)).then(
                    function (response) {
                        console.log(response);
                        return response.data;
                    });
            },
        };
    }]);
