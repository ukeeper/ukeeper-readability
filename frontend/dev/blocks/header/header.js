$(function() {
	var $currentPage = $('.menu__item a[href="' + location.pathname + '"]');

	$currentPage.parent().html($currentPage.text());
});