// Atlas configuration for Ent integration
env {
  name = atlas.env
  src  = "ent://ent/schema"
  dev  = "docker://postgres/15/dev?search_path=public"
  
  migration {
    dir = "file://migrations"
  }
  
  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}
