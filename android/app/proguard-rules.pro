# gomobile: the Go runtime is started from native code that looks classes and methods up by name.
-keep class go.** { *; }
-keep class nasc.** { *; }

# The page calls these through addJavascriptInterface (window.NascNative).
-keepclassmembers class app.nasc.MainActivity$LockBridge {
    @android.webkit.JavascriptInterface <methods>;
}
