{ ... }:
{
  projectRootFile = "flake.nix";
  programs.gofmt.enable = true;
  programs.prettier.enable = true;
  programs.nixfmt.enable = true;
}
