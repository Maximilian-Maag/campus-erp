// Karma configuration (Angular 20 / @angular/build:karma).
//
// Providing a custom `karmaConfig` REPLACES the builder's built-in config, so this
// mirrors it (frameworks / plugins / reporters) and adds a headless launcher that
// also works in containers and CI running as root (--no-sandbox).
module.exports = function (config) {
  config.set({
    basePath: '',
    frameworks: ['jasmine'],
    plugins: [
      require('karma-jasmine'),
      require('karma-chrome-launcher'),
      require('karma-jasmine-html-reporter'),
      require('karma-coverage'),
    ],
    jasmineHtmlReporter: { suppressAll: true },
    client: { clearContext: true },
    reporters: ['progress', 'kjhtml'],
    browsers: ['ChromeHeadlessNoSandbox'],
    customLaunchers: {
      ChromeHeadlessNoSandbox: {
        base: 'ChromeHeadless',
        flags: ['--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage'],
      },
    },
    restartOnFileChange: true,
  });
};
