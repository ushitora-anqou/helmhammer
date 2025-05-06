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
    #jrsonnet = pkgs.jrsonnet.overrideAttrs (finalAttrs: previousAttrs: {
    #  src = pkgs.fetchFromGitHub {
    #    owner = "CertainLach";
    #    repo = "jrsonnet";
    #    rev = "0e1ae581969b0ab6489a867723470007f0b92472";
    #    sha256 = "sha256-dm62UkL8lbvU3Ftjj6K5ziZGuHpFyLUzyTg9x/+no54=";
    #  };
    #  # cf. https://discourse.nixos.org/t/overriding-version-on-rust-based-package/57445/2
    #  cargoDeps = pkgs.rustPlatform.fetchCargoVendor {
    #    inherit (finalAttrs) pname src version;
    #    hash = finalAttrs.cargoHash;
    #  };
    #  cargoHash = "sha256-ZHmdlqakucapzXJz6L7ZJpmvqTutelN8qkWAD4uDJr8";
    #  postInstall = "ln -s $out/bin/jrsonnet $out/bin/jsonnet";
    #});
  in {
    devShells."${system}" = rec {
      default = pkgs.mkShell {
        packages = [go-jsonnet] ++ (with pkgs; [alejandra kubernetes-helm jq yq-go]);
      };
    };
  };
}
