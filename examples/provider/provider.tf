terraform {
  required_providers {
    dimdams = {
      source = "dimdam-s/dimdams"
    }
  }
}

# The API key comes from the DIMDAMS_API_KEY environment variable.
provider "dimdams" {}
