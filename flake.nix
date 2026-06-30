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
          version = "0.4.0";
        in
        {
          default = pkgs.buildGoModule {
            pname = "thop";
            inherit version;

            src = self;
            vendorHash = "sha256-epeR/QGb/sWvBAKTqACXPnuFzmA7OpZVyzUHIdt/V9A=";

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
