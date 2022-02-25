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