$(function() {
	var $login = $('#login');

	if ($login.length) {
		$login.submit(function(event) {
			$('.login__error', $login).hide();

			var login = $(this).find('input[name=login]').val(),
				pass = $(this).find('input[name=password]').val()

			if (login && pass) { 
				var request = new XMLHttpRequest();   

				request.open('POST', APIPath + '/auth', true, login, pass);                                                                                                                               
				request.setRequestHeader('Authorization', 'Basic ' + btoa(login + ':' + pass));

			    request.onreadystatechange = function(event) {  
			        if (request.readyState === 4) {  
			            if (request.status !== 200) {  
							$('#login .login__error').show();  
			            } else {  
							localStorage.setItem('login', login);
							localStorage.setItem('password', pass);
							
							var back = getParameterByName('back');

							if (back) {
								location.href = back;
							} else {
								location.href = '/';
							}
			            }  
			        }  
			    }; 

			    request.send();    
			} 

			return false;
		});
	}
});