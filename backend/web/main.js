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
$(function() {
	var $currentPage = $('.menu__item a[href="' + location.pathname + '"]');

	$currentPage.parent().html($currentPage.text());
});
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
function loadPreview(url, $button) {
	$button
		.addClass('form__button_loading')
		.attr('disabled', true);

	$.ajax({
		url: APIPath + '/extract',
		type: 'POST',
		async: true,
		headers: authHeaders,
		data: JSON.stringify({
			url: url
		})
	})
	.done(function(json) {
		var $preview = $('#preview'),
			map = {
				title:        	'.preview__title',
				content:      	'.preview__content',
				rich_content: 	'.preview__rich-content',
				excerpt:      	'.preview__excerpt',
			};

		for (var prop in json) {
			$(map[prop], $preview).html(json[prop]);
		}

		$preview.show();
		$('html,body').animate({
			scrollTop: $preview.offset().top
		}, 300);
	})
	.fail(function(response) {
		console.log("error while loading preview");
		console.log(response);
	})
	.always(function(response) {
		$button
			.removeClass('form__button_loading')
			.attr('disabled', false);

		$('.form__loader').addClass('form__loader_hidden');
	});
}


$(function() {
	var $rule = $('#rule'),
		$newTestURLs = $('.rule__new-test-url', $rule),
		$testURLs = $('.rule__test-urls', $rule);

	if ($rule.length) {
		var id = getParameterByName('id'),
			map = {
				domain: '.rule__domain',
				content: '.rule__content',
				author: '.rule__author',
				match_url: '.rule__match-urls',
				excludes: '.rule__excludes',
				test_urls: '.rule__test-urls'
			},
			textBoxes = [ 'match_url', 'excludes', 'test_urls'];

		if (id.length) {
			$.ajax({
				url: APIPath + '/rule/' + id,
				type: 'GET',
				dataType: 'json'
			})
			.done(function(json) {
				$rule.data('data', json);

				for (var prop in map) {
					var val = json[prop];

					if (textBoxes.indexOf(prop) != -1) {
						val = val.join('\n');
					}

					$(map[prop], $rule).val(val);
				}
			})
			.fail(function(response) {
				console.log("error while loading rule");
				console.log(response);
			});
		}

		$('.rule__button-save', $rule).click(function(event) {
			var json = {};
			var data = $rule.data('data');

			for (var prop in map) {
				var val = $(map[prop], $rule).val();

				if (textBoxes.indexOf(prop) != -1) {
					val = val.split('\n');
				}

				json[prop] = val;
			}

			if (id.length) {
				json.enabled = data.enabled;
				json.id = data.id;
				json.user = data.user;
			}

			$.ajax({
				url: APIPath + '/rule',
				type: 'POST',
				async: true,
				headers: authHeaders,
				data: JSON.stringify(json)
			})
			.done(function() {
				location.href = '/';
			})
			.fail(function(response) {
				console.log("error while saving the rule");
				console.log(response);
			});
		});

		$('.rule__button-show', $rule).click(function() {
			var $tip = $(this).siblings('.form__button-tip')
				$loader = $('.form__loader', $rule),
				index = $testURLs
						.val()
						.substr(0, $testURLs[0].selectionStart)
						.split('\n')
						.length,
				lines = $testURLs
						.val()
						.split('\n'),
				url = lines[index - 1];

			if (url.length) {
				$tip.hide();
				$loader.removeClass('form__loader_hidden');
				loadPreview(url, $(this)); // preview.js
			} else {
				$tip.text('Выбрана пустая строка').show();
			}
		});

		$('.form__input', $rule).keydown(function(e) {
			if (e.ctrlKey && e.keyCode == 13) {
				$('.rule__button-save', $rule).click();
			}
		});
	}
});
$(function() {
	if ($('#rules__list').length) {
		loadRules();
	};
});

function loadRules() {
	$('#rules__list').html('');

	$.ajax({
		url: APIPath + '/rules',
		type: 'GET',
		dataType: 'json'
	})
	.done(function(json) {
		var $row;

		for (var i = 0; i < json.length; i++) {
			$row = $('<tr/>', {
				class: 'rules__row'
			}).data('data', json[i]);

			$('<td/>', {
				class: 'rules__domain-cell',
				html: '<a href="/edit/?id=' + json[i].id + '" class="link">' + json[i].domain + '</a>'
			}).appendTo($row);

			$('<td/>', {
				class: 'rules__content-cell',
				text: json[i].content
			}).appendTo($row);

			$('<td/>', {
				class: 'rules__enabled-cell',
				html: '<input class="rules__enabled" type="checkbox" ' + ((json[i].enabled) ? 'checked' : '') + '>'
			}).appendTo($row);

			if (!json[i].enabled) {
				$row.addClass('rules__row_disabled');
			}

			$('#rules__list').append($row);
		};


		$('.rules__enabled').change(function(event) {
			event.preventDefault();

			toggleRule($(this).closest('tr'));
		});
	})
	.fail(function(response) {
		console.log("error while loading rules");
		console.log(response);
	});
}

function toggleRule($row) {
	var data = $row.data('data');

	data.enabled = !data.enabled;

	if (data.enabled) {
		$.ajax({
			url: APIPath + '/rule',
			type: 'POST',
			async: true,
			headers: authHeaders,
			data: JSON.stringify(data)
		})
		.done(function() {
			$row.data('data', data);
			$row.removeClass('rules__row_disabled');
		})
		.fail(function(response) {
			console.log("error while enabling the rule");
			console.log(response);
		});
	} else {
		$.ajax({
			url: APIPath + '/rule/' + data.id,
			type: 'DELETE',
			async: true,
			headers: authHeaders
		})
		.done(function() {
			$row.data('data', data);
			$row.addClass('rules__row_disabled');
		})
		.fail(function(response) {
			console.log("error while disabling the rule");
			console.log(response);
		});
	}
}