{
  description = "Fuzzy tmux session manager with frecency ranking";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      forAllSystems = nixpkgs.lib.genAttrs nixpkgs.lib.systems.flakeExposed;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          version = "0.2.1-test";
        in
        {
          default = pkgs.buildGoModule {
            pname = "thop";
            inherit version;

            src = self;
            vendorHash = "sha256-oTm69m9U7XMcVn86Gr4Rm88plLOQ0nk+4mcLk4NUuV0=";

            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];

            meta = with pkgs.lib; {
              description = "Fuzzy tmux session manager with frecency ranking";
              homepage = "https://github.com/rwilgaard/thop";
              license = licenses.mit;
              mainProgram = "thop";
            };
          };
        }
      );
    };
}
