$(function() {
	$('#logout').click(function(event) {
		event.preventDefault();

		var request = new XMLHttpRequest();   

		request.open('POST', APIPath + '/auth', false, 'harry', 'colloportus');                                                                                                                               
	    
	    try {
		    request.send();    
			    
	        if (request.readyState === 4) {  
				localStorage.removeItem('login');
				localStorage.removeItem('password');

				location.href = '/login/';
	        }  
	    } catch (err) {
	    	localStorage.removeItem('login');
			localStorage.removeItem('password');

			location.href = '/login/';
	    }
	});
});