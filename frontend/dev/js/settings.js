var APIPath = '/api',
	login = localStorage.getItem('login'),
	password = localStorage.getItem('password'),
	authHeaders = {
		'Authorization': 'Basic ' + btoa(login + ':' + password)
	},
	isAdmin = login && password;

function getParameterByName(name) {
    name = name.replace(/[\[]/, "\\[").replace(/[\]]/, "\\]");
    var regex = new RegExp("[\\?&]" + name + "=([^&#]*)"),
        results = regex.exec(location.search);

    return results === null ? "" : decodeURIComponent(results[1].replace(/\+/g, " "));
}