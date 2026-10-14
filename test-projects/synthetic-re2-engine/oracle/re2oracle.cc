// RE2 oracle helper. It is test tooling only and is never linked by rex.
#include <iostream>
#include <re2/re2.h>
#include <string>

int main(int argc, char** argv) {
  if (argc != 3) return 2;
  const std::string pattern(argv[1]);
  const std::string text(argv[2]);
  re2::RE2 re(pattern);
  if (!re.ok()) {
    std::cout << "ERROR\n";
    return 0;
  }
  re2::StringPiece groups[1];
  if (!re.Match(text, 0, text.size(), re2::RE2::UNANCHORED, groups, 1)) {
    std::cout << "NO\n";
    return 0;
  }
  std::cout << "MATCH " << (groups[0].data() - text.data()) << " "
            << groups[0].size() << "\n";
}
