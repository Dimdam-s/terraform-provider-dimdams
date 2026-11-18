terraform {
  required_providers {
    dimdams = {
      source = "dimdams/dimdams"
    }
  }
}

# The API key comes from the DIMDAMS_API_KEY environment variable.
provider "dimdams" {}
