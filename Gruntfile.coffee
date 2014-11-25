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
      resources:
        files: [
          {expand: true, cwd: 'src/resources/', src: ['**'], dest: 'out/web/'}
        ]
    typescript:
      base:
        src: ['src/ts/*.ts']
        dest: 'out/web/wetube.js'
        options:
          target: 'es5'
          basePath: 'src/ts/'
          sourceMap: true
          declaration: false
    go:
      wetube:
        root: 'src/go/'
        output: 'out/'
        run_files: ['wetube.go']
    open:
      dev:
        path: 'http://localhost:9191/index.html'

  grunt.registerTask 'default', ['mkdir', 'copy:resources', 'typescript', 'go:run:wetube']

