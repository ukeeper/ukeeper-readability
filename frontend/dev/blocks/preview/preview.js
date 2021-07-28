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
	
