module.exports = (grunt) ->

  grunt.loadNpmTasks 'grunt-contrib-clean'
  grunt.loadNpmTasks 'grunt-mkdir'
  grunt.loadNpmTasks 'grunt-contrib-copy'
  grunt.loadNpmTasks 'grunt-typescript'
  grunt.loadNpmTasks 'grunt-go'
  grunt.loadNpmTasks 'grunt-open'

  grunt.initConfig
    pkg: grunt.file.readJSON("package.json")
    clean: ['out/']
    mkdir:
      all:
        options:
          create: ['out', 'out/web']
    copy:
      gosrc: # Copy the Go files to the output dir because Go is picky about paths...
        files: [
          {expand: true, cwd: 'src/', src: ['**'], dest: 'out/'}
        ]
    typescript:
      base:
        src: ['ts/**/*.ts']
        dest: 'out/web/wetube.js'
        options:
          target: 'es5'
          basePath: 'ts/'
          sourceMap: true
          declaration: false
    go:
      wetube_src:
        root: 'src/'
        output: 'out/'
        run_files: ['wetube/wetube.go']
      wetube_out:
        root: 'out/'
        output: 'out/'
        run_files: ['wetube/wetube.go']
    open:
      wetube:
        path: 'http://localhost:9191/index.html'
        options:
          openOn: 'browserLaunch'

  grunt.registerTask 'delayBrowserLaunch', ->
    setTimeout ->
      grunt.event.emit("browserLaunch")
    , 3000

  grunt.registerTask 'default', [
    'mkdir'
    'go:fmt:wetube_src'
    'copy:gosrc'
    'typescript'
    'open:wetube'
    'delayBrowserLaunch'
    'go:run:wetube_out'
  ]
