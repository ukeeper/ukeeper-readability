const APIPath = '/api';
const authHeaders = {
    'Authorization': 'Basic ' + btoa(localStorage.getItem('login') + ':' + localStorage.getItem('password'))
};

function getParameterByName(name) {
    name = name.replace(/[\[]/, "\\[").replace(/[\]]/, "\\]");
    const regex = new RegExp("[\\?&]" + name + "=([^&#]*)");
    const results = regex.exec(location.search);

    return results === null ? "" : decodeURIComponent(results[1].replace(/\+/g, " "));
}

document.addEventListener('DOMContentLoaded', () => {
    const currentPage = document.querySelector('.menu__item a[href="' + location.pathname + '"]');

    if (currentPage) {
        currentPage.parentNode.innerHTML = currentPage.textContent;
    }

    const loginForm = document.getElementById('login');

    if (loginForm) {
        loginForm.addEventListener('submit', (event) => {
            event.preventDefault();
            document.querySelector('.login__error').style.display = 'none';

            const login = loginForm.querySelector('input[name=login]').value;
            const pass = loginForm.querySelector('input[name=password]').value;

            if (login && pass) {
                const request = new XMLHttpRequest();

                request.open('POST', APIPath + '/auth', true);
                request.setRequestHeader('Authorization', 'Basic ' + btoa(login + ':' + pass));

                request.onreadystatechange = function () {
                    if (request.readyState === 4) {
                        if (request.status !== 200) {
                            document.querySelector('#login .login__error').style.display = 'block';
                        } else {
                            localStorage.setItem('login', login);
                            localStorage.setItem('password', pass);

                            const back = getParameterByName('back');
                            location.href = back ? back : '/';
                        }
                    }
                };

                request.send();
            }
        });
    }

    const logoutButton = document.getElementById('logout');

    if (logoutButton) {
        logoutButton.addEventListener('click', (event) => {
            event.preventDefault();

            const request = new XMLHttpRequest();

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
    }

    function loadPreview(url, button) {
        button.classList.add('form__button_loading');
        button.disabled = true;

        const request = new XMLHttpRequest();
        request.open('POST', APIPath + '/extract', true);
        request.setRequestHeader('Content-Type', 'application/json;charset=UTF-8');
        request.setRequestHeader('Authorization', authHeaders.Authorization);

        request.onreadystatechange = function () {
            if (request.readyState === 4) {
                if (request.status === 200) {
                    const json = JSON.parse(request.responseText);
                    const preview = document.getElementById('preview');
                    const map = {
                        title: '.preview__title',
                        content: '.preview__content',
                        rich_content: '.preview__rich-content',
                        excerpt: '.preview__excerpt',
                    };

                    for (const prop in map) {
                        if (json[prop]) {
                            preview.querySelector(map[prop]).innerHTML = json[prop];
                        }
                    }

                    preview.style.display = 'block';
                    window.scrollTo({
                        top: preview.offsetTop,
                        behavior: 'smooth'
                    });
                } else {
                    console.log("error while loading preview");
                    console.log(request.responseText);
                }

                button.classList.remove('form__button_loading');
                button.disabled = false;
                document.querySelector('.form__loader').classList.add('form__loader_hidden');
            }
        };

        request.send(JSON.stringify({url: url}));
    }

    const rule = document.getElementById('rule');
    const testURLs = rule ? rule.querySelector('.rule__test-urls') : null;

    if (rule) {
        const id = getParameterByName('id');
        const map = {
            domain: '.rule__domain',
            content: '.rule__content',
            author: '.rule__author',
            match_url: '.rule__match-urls',
            excludes: '.rule__excludes',
            test_urls: '.rule__test-urls'
        };
        const textBoxes = ['match_url', 'excludes', 'test_urls'];

        if (id) {
            const request = new XMLHttpRequest();
            request.open('GET', APIPath + '/rule/' + id, true);
            request.responseType = 'json';

            request.onload = function () {
                if (request.status === 200) {
                    const json = request.response;
                    rule.dataset.data = JSON.stringify(json);

                    for (const prop in map) {
                        let val = json[prop];

                        if (textBoxes.includes(prop)) {
                            val = val.join('\n');
                        }

                        rule.querySelector(map[prop]).value = val;
                    }
                } else {
                    console.log("error while loading rule");
                    console.log(request.responseText);
                }
            };

            request.send();
        }

        rule.querySelector('.rule__button-save').addEventListener('click', function () {
            let json = {};
            let data = {};

            if (rule.dataset.data) {
                data = JSON.parse(rule.dataset.data);
            }

            for (const prop in map) {
                let val = rule.querySelector(map[prop]).value;

                if (textBoxes.includes(prop)) {
                    val = val.split('\n');
                }

                json[prop] = val;
            }

            if (id) {
                json.enabled = data.enabled;
                json.id = data.id;
                json.user = data.user;
            }

            const request = new XMLHttpRequest();
            request.open('POST', APIPath + '/rule', true);
            request.setRequestHeader('Content-Type', 'application/json;charset=UTF-8');
            request.setRequestHeader('Authorization', authHeaders.Authorization);

            request.onreadystatechange = function () {
                if (request.readyState === 4) {
                    if (request.status === 200) {
                        location.href = '/';
                    } else {
                        console.log("error while saving the rule");
                        console.log(request.responseText);
                    }
                }
            };

            request.send(JSON.stringify(json));
        });

        rule.querySelector('.rule__button-show').addEventListener('click', function () {
            const tip = this.nextElementSibling;
            const loader = document.querySelector('.form__loader');
            const index = testURLs.value.substring(0, testURLs.selectionStart).split('\n').length;
            const lines = testURLs.value.split('\n');
            const url = lines[index - 1];

            if (url.length) {
                tip.style.display = 'none';
                loader.classList.remove('form__loader_hidden');
                loadPreview(url, this);
            } else {
                tip.textContent = 'Выбрана пустая строка';
                tip.style.display = 'block';
            }
        });
    }
});
