fn main() {
    println!("cargo:rustc-link-lib=static=stdc++");
    println!("cargo:rustc-link-search=/usr/lib");
}
