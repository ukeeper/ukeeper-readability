/* requires */

import url from "gulp-css-url-adjuster";
import uglify from "gulp-uglify";
import fileinclude from "gulp-file-include";
import concat from "gulp-concat";
import cache from "gulp-cached";
import browserSync from "browser-sync";
import ifelse from "gulp-if";
import util from "gulp-util";
import rename from "gulp-rename";
import {deleteAsync} from 'del';
import addsrc from "gulp-add-src";
import importOnce from "node-sass-import-once";
import autoprefixer from "autoprefixer";
import postcss from "gulp-postcss";
import gulp from "gulp";
import gulpSass from 'gulp-sass';
import sassCompiler from 'sass';
const sass = gulpSass(sassCompiler);

/* paths */

const mask = {
		html: ['dev/html/**/*', 'dev/includes/*.html', 'dev/blocks/**/*.html'],
		scss: 'dev/blocks/**/*.scss',
		css: 'dev/css/**/*.css',
		js_f: 'dev/js/**/*',
		js_b: 'dev/blocks/**/*.js',
		images: 'dev/blocks/**/*.{jpg,png,gif,svg}',
		files: 'dev/files/**/*',
		fonts: 'dev/fonts/**/*.{eot,svg,ttf,woff,woff2}',
		main: ['public/**', '!public'],
	},
	input = {
		html: 'dev/html/**/*.html',
		css: 'dev/css',
		scss: 'dev/blocks/main.scss',
	},
	output = {
		main: 'public',
		js: 'public/js',
		css: 'public/css',
		images: 'public/images',
		files: 'public/files',
		fonts: 'public/fonts'
	},
	isProduction = (util.env.type === 'production'),
	isDeploy = (util.env.type === 'deploy');

gulp.task('html', function(cb) {
	gulp.src(input.html)
		.pipe(fileinclude())
		.on('error', util.log)
		.pipe(cache('htmling'))
		.pipe(gulp.dest(output.main))
		.pipe(browserSync.stream());
	cb();
});

gulp.task('scss', function(cb) {
	gulp.src(input.scss)
		.pipe(sass({importer: importOnce}, false).on('error', util.log))
		.pipe(gulp.dest(input.css))
	cb();
});

gulp.task('css', function(cb) {
	gulp.src(mask.css)
		.pipe(cache('cssing'))
		.pipe(postcss([autoprefixer({browsers: ["> 1%"]})]))
		.pipe(url({replace: [/^i-/, '../images/i-']}))
		.pipe(url({replace: [/^f-/, '../fonts/f-']}))
		.pipe(gulp.dest(output.css))
		.pipe(browserSync.stream());
	cb();
});

gulp.task('images', function(cb) {
	gulp.src(mask.images)
		.pipe(cache('imaging'))
		.pipe(rename({dirname: ''}))
		.pipe(gulp.dest(output.images))
		.pipe(browserSync.stream());
	cb();
});

gulp.task('files', function(cb) {
	gulp.src(mask.files)
		.pipe(gulp.dest(output.files))
		.pipe(browserSync.stream());
	cb();
});

gulp.task('js', function(cb) {
	gulp.src(mask.js_f)
		.pipe(concat('main.js'))
		.pipe(addsrc(mask.js_b))
		.pipe(concat('main.js'))
		.pipe(cache('jsing'))
		.pipe(ifelse(isProduction || isDeploy, uglify()))
		.pipe(gulp.dest(output.js))
		.pipe(browserSync.stream());
	cb();
});

gulp.task('fonts', function(cb) {
	gulp.src(mask.fonts)
		.pipe(rename({dirname: ''}))
		.pipe(gulp.dest(output.fonts))
		.pipe(browserSync.stream());
	cb();
});

gulp.task('server', function(cb) {
	browserSync.init({
		server: output.main,
		open: false,
		browser: "browser",
		reloadOnRestart: true,
		online: true
	});
	cb();
});

gulp.task('watch', function(cb) {
	gulp.watch(mask.html, gulp.series('html'));
	gulp.watch(mask.scss, gulp.series('scss'));
	gulp.watch(mask.css, gulp.series('css'));
	gulp.watch([mask.js_f, mask.js_b], gulp.series('js'));
	gulp.watch(mask.images, gulp.series('images'));
	gulp.watch(mask.files, gulp.series('files'));
	gulp.watch(mask.fonts, gulp.series('fonts'));
	cb();
});

gulp.task('clean', function(cb) {
	deleteAsync(mask.main);
	cb();
});

gulp.task('build', gulp.series('html', 'scss', 'css', 'js', 'images', 'files', 'fonts'));
gulp.task('default', gulp.series('build', 'server', 'watch'));
