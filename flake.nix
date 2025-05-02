{
  description = "Helmhammer";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs = {
    self,
    nixpkgs,
  }: let
    system = "x86_64-linux";
    pkgs = nixpkgs.legacyPackages."${system}";
    lib = nixpkgs.lib;

    #go-jsonnet = pkgs.go-jsonnet;
    go-jsonnet = pkgs.go-jsonnet.overrideAttrs (finalAttrs: previousAttrs: {
      src = pkgs.fetchFromGitHub {
        owner = "google";
        repo = "go-jsonnet";
        rev = "2a3f4afd6af061d8b22a01e878184b04e42ca011";
        sha256 = "sha256-Tquk8idbUYFEKLpnKRYY8cV0YgTOdti7mT9TUnu4Kx0=";
      };
      vendorHash = "sha256-Uh2rAXdye9QmmZuEqx1qeokE9Z9domyHsSFlU7YZsZw=";
    });
  in {
    devShells."${system}" = rec {
      default = pkgs.mkShell {
        packages = [go-jsonnet] ++ (with pkgs; [alejandra kubernetes-helm jq yq-go]);
      };
    };
  };
}
