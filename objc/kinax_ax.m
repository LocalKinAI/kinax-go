// kinax-go — macOS Accessibility (AX) API shim.
//
// Exposes a minimal C ABI for Go purego callers to navigate and
// manipulate the system-wide UI tree via AXUIElement. This is the
// same API that VoiceOver, Accessibility Inspector, and screen
// readers use; it's also the foundation for a swarm of UI automation
// agents (observing what the user is doing, clicking buttons on
// their behalf, reading arbitrary app state).
//
// Model: every AXUIElement is returned to Go as an opaque handle
// (an autoreleased retain count on the CFTypeRef). Go is responsible
// for calling kinax_element_release when done. All handles are
// valid until explicitly released — never leaked on process exit
// because CF memory goes away with the process anyway.
//
// Permission note: macOS 10.15+ requires the invoking binary to be
// listed in System Settings → Privacy & Security → Accessibility
// for AXUIElementCopyAttributeValue to succeed. Without permission,
// every call returns kAXErrorCannotComplete or similar. Callers check
// permission via kinax_ax_trusted.

#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#import <ApplicationServices/ApplicationServices.h>
#import <AppKit/AppKit.h>

// ─── Exported C ABI ──────────────────────────────────────────

#ifdef __cplusplus
extern "C" {
#endif

// Trust check. prompt=1 shows the system dialog on first call.
int32_t kinax_ax_trusted(int32_t prompt);

// Return an opaque handle to the system-wide element (the root of
// the UI tree). Always succeeds. Owner must call kinax_element_release.
uintptr_t kinax_system_wide(void);

// Return an opaque handle to the focused application (the one with
// keyboard focus right now). Returns 0 if none.
uintptr_t kinax_focused_application(void);

// Return a handle to the AXUIElement representing the running app
// with the given PID.
uintptr_t kinax_app_by_pid(int32_t pid);

// Return the PID of the frontmost application. Returns 0 on error.
int32_t kinax_frontmost_pid(void);

// Return the PID of the first running app with the given bundle ID.
// Returns 0 if no such app is running.
int32_t kinax_pid_by_bundle(const char *bundle_id);

// Return the element at the given screen coordinates (global, top-
// left origin). Returns 0 if nothing is there.
uintptr_t kinax_element_at_point(double x, double y);

// Release a handle. The underlying CFTypeRef is CFRelease'd; after
// this call, the handle MUST NOT be used.
void kinax_element_release(uintptr_t handle);

// Copy a string attribute. Returns 0 on success (and copies up to
// buflen-1 UTF-8 bytes + NUL terminator into buf), -1 on error, or
// the required buffer length (including NUL) if it would overflow.
int32_t kinax_element_attr_string(uintptr_t handle, const char *attr,
                                  char *buf, int32_t buflen);

// Copy an integer attribute. Returns 0 on success (writes *out),
// non-zero otherwise.
int32_t kinax_element_attr_int(uintptr_t handle, const char *attr,
                               int64_t *out);

// Copy a boolean attribute. Returns 0 on success (writes *out to
// 0 or 1), non-zero otherwise.
int32_t kinax_element_attr_bool(uintptr_t handle, const char *attr,
                                int32_t *out);

// Copy an element-valued attribute (e.g. AXFocusedWindow) and
// return a new handle. Returns 0 if absent or wrong type.
uintptr_t kinax_element_attr_element(uintptr_t handle, const char *attr);

// Copy an AXValue CGPoint attribute (e.g. AXPosition). Returns 0 on
// success (writes x/y), non-zero otherwise.
int32_t kinax_element_attr_point(uintptr_t handle, const char *attr,
                                 double *x_out, double *y_out);

// Copy an AXValue CGSize attribute (e.g. AXSize).
int32_t kinax_element_attr_size(uintptr_t handle, const char *attr,
                                double *w_out, double *h_out);

// Copy an array-of-elements attribute (e.g. AXChildren, AXWindows).
// On success, writes up to *count element handles into handles[] and
// updates *count to the total number of elements (may exceed the
// buffer — in which case only the first N are filled). Returns 0 on
// success.
int32_t kinax_element_attr_element_array(uintptr_t handle, const char *attr,
                                         uintptr_t *handles, int32_t *count);

// List attribute names as a JSON string array ["AXRole", "AXTitle", ...].
// Same semantics as kinax_element_attr_string: returns 0 on success.
int32_t kinax_element_attribute_names(uintptr_t handle,
                                      char *buf, int32_t buflen);

// Copy multiple attribute values in one IPC round-trip via
// AXUIElementCopyMultipleAttributeValues. attrs_json is a JSON array of
// attribute name strings, e.g. '["AXRole","AXTitle","AXEnabled"]'.
//
// On success (return 0), writes a JSON object to buf with one key per
// successfully-fetched attribute:
//
//   {"AXRole":"AXButton","AXTitle":"Save","AXEnabled":true}
//
// Missing or unsupported attributes are simply absent from the object.
// Element-valued attributes (AXChildren, AXMainWindow, AXFocusedWindow,
// etc.) are also omitted — callers must use the dedicated
// kinax_element_attr_element / kinax_element_attr_element_array entry
// points to materialize handles.
//
// Same buflen-overflow behavior as kinax_element_attr_string: returns
// the required buffer size (including NUL) if the result wouldn't fit.
//
// Performance note: a tree dump that previously made N×M synchronous
// AX IPC round-trips now makes N — one IPC per node, all attributes
// at once. Measured 2-5× speedup on dense apps (Cursor / Slack / Xcode).
int32_t kinax_element_attr_many(uintptr_t handle, const char *attrs_json,
                                char *buf, int32_t buflen);

// List action names as a JSON string array ["AXPress", "AXShowMenu", ...].
int32_t kinax_element_action_names(uintptr_t handle,
                                   char *buf, int32_t buflen);

// Perform the named action (e.g. "AXPress"). Returns 0 on success.
int32_t kinax_element_perform(uintptr_t handle, const char *action);

// Set a string-valued attribute. Returns 0 on success. Typically used
// for AXValue on text fields.
int32_t kinax_element_set_string(uintptr_t handle, const char *attr,
                                 const char *value);

// Set a boolean-valued attribute.
int32_t kinax_element_set_bool(uintptr_t handle, const char *attr,
                               int32_t value);

#ifdef __cplusplus
}
#endif

// ─── Internal helpers ────────────────────────────────────────

// CFBridge a CFTypeRef out as an opaque uintptr_t handle with an
// extra retain. Go will CFRelease via kinax_element_release.
static uintptr_t export_element(AXUIElementRef el) {
    if (!el) return 0;
    CFRetain(el);
    return (uintptr_t)el;
}

static AXUIElementRef import_element(uintptr_t handle) {
    return (AXUIElementRef)handle;
}

static NSString *nsstr(const char *c) {
    if (!c) return nil;
    return [NSString stringWithUTF8String:c];
}

// Write a UTF-8 string into buf/buflen, mimicking snprintf semantics
// but returning what kinax_element_attr_string uses:
//   0  on success
//   n  (> 0) required buffer size (including NUL) if it would overflow
//   -1 on internal error
static int32_t write_string_result(NSString *s, char *buf, int32_t buflen) {
    if (!s) {
        if (buf && buflen > 0) buf[0] = 0;
        return 0;
    }
    const char *utf8 = [s UTF8String];
    if (!utf8) return -1;
    size_t len = strlen(utf8);
    if ((int32_t)len + 1 > buflen) {
        return (int32_t)len + 1; // caller should resize
    }
    memcpy(buf, utf8, len);
    buf[len] = 0;
    return 0;
}

// Encode an NSArray<NSString *> as a compact JSON string array.
static NSString *json_string_array(NSArray<NSString *> *items) {
    if (!items || items.count == 0) return @"[]";
    NSError *err = nil;
    NSData *d = [NSJSONSerialization dataWithJSONObject:items options:0 error:&err];
    if (err || !d) return @"[]";
    return [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
}

// ─── Trust ───────────────────────────────────────────────────

int32_t kinax_ax_trusted(int32_t prompt) {
    if (prompt) {
        NSDictionary *opts = @{
            (__bridge NSString *)kAXTrustedCheckOptionPrompt: @YES
        };
        return AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)opts) ? 1 : 0;
    }
    return AXIsProcessTrusted() ? 1 : 0;
}

// ─── Root / app handles ──────────────────────────────────────

uintptr_t kinax_system_wide(void) {
    AXUIElementRef el = AXUIElementCreateSystemWide();
    if (!el) return 0;
    // We created it — ownership is on the caller. Don't double-retain.
    return (uintptr_t)el;
}

uintptr_t kinax_focused_application(void) {
    pid_t pid = kinax_frontmost_pid();
    if (pid == 0) return 0;
    AXUIElementRef el = AXUIElementCreateApplication(pid);
    if (!el) return 0;
    return (uintptr_t)el;
}

uintptr_t kinax_app_by_pid(int32_t pid) {
    if (pid <= 0) return 0;
    AXUIElementRef el = AXUIElementCreateApplication((pid_t)pid);
    if (!el) return 0;
    return (uintptr_t)el;
}

int32_t kinax_frontmost_pid(void) {
    NSRunningApplication *app = [[NSWorkspace sharedWorkspace] frontmostApplication];
    if (!app) return 0;
    return (int32_t)[app processIdentifier];
}

int32_t kinax_pid_by_bundle(const char *bundle_id) {
    NSString *bid = nsstr(bundle_id);
    if (!bid) return 0;
    NSArray<NSRunningApplication *> *apps =
        [NSRunningApplication runningApplicationsWithBundleIdentifier:bid];
    if (apps.count == 0) return 0;
    return (int32_t)[apps[0] processIdentifier];
}

uintptr_t kinax_element_at_point(double x, double y) {
    AXUIElementRef sys = AXUIElementCreateSystemWide();
    if (!sys) return 0;
    AXUIElementRef out = NULL;
    AXError err = AXUIElementCopyElementAtPosition(sys, (float)x, (float)y, &out);
    CFRelease(sys);
    if (err != kAXErrorSuccess || !out) return 0;
    return (uintptr_t)out;
}

void kinax_element_release(uintptr_t handle) {
    if (handle == 0) return;
    CFRelease((CFTypeRef)handle);
}

// ─── Attribute readers ───────────────────────────────────────

int32_t kinax_element_attr_string(uintptr_t handle, const char *attr,
                                  char *buf, int32_t buflen) {
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name || !buf || buflen <= 0) return -1;

    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) {
        buf[0] = 0;
        return -1;
    }

    NSString *s = nil;
    CFTypeID tid = CFGetTypeID(value);
    if (tid == CFStringGetTypeID()) {
        s = (__bridge NSString *)value;
    } else if (tid == CFNumberGetTypeID()) {
        s = [(__bridge NSNumber *)value stringValue];
    } else if (tid == CFBooleanGetTypeID()) {
        s = CFBooleanGetValue(value) ? @"true" : @"false";
    } else {
        // Fall back to CF description — informative but not guaranteed stable.
        s = [(__bridge id)value description];
    }
    int32_t rc = write_string_result(s, buf, buflen);
    CFRelease(value);
    return rc;
}

int32_t kinax_element_attr_int(uintptr_t handle, const char *attr, int64_t *out) {
    if (!out) return -1;
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return -1;

    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) return -1;
    int32_t rc = -1;
    if (CFGetTypeID(value) == CFNumberGetTypeID()) {
        long long v = 0;
        if (CFNumberGetValue(value, kCFNumberLongLongType, &v)) {
            *out = (int64_t)v;
            rc = 0;
        }
    }
    CFRelease(value);
    return rc;
}

int32_t kinax_element_attr_bool(uintptr_t handle, const char *attr, int32_t *out) {
    if (!out) return -1;
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return -1;

    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) return -1;
    int32_t rc = -1;
    if (CFGetTypeID(value) == CFBooleanGetTypeID()) {
        *out = CFBooleanGetValue(value) ? 1 : 0;
        rc = 0;
    } else if (CFGetTypeID(value) == CFNumberGetTypeID()) {
        int i = 0;
        if (CFNumberGetValue(value, kCFNumberIntType, &i)) {
            *out = i ? 1 : 0;
            rc = 0;
        }
    }
    CFRelease(value);
    return rc;
}

uintptr_t kinax_element_attr_element(uintptr_t handle, const char *attr) {
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return 0;
    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) return 0;
    uintptr_t out = 0;
    if (CFGetTypeID(value) == AXUIElementGetTypeID()) {
        out = (uintptr_t)value; // transfer ownership; don't CFRelease.
    } else {
        CFRelease(value);
    }
    return out;
}

int32_t kinax_element_attr_point(uintptr_t handle, const char *attr,
                                 double *x_out, double *y_out) {
    if (!x_out || !y_out) return -1;
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return -1;
    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) return -1;
    int32_t rc = -1;
    if (CFGetTypeID(value) == AXValueGetTypeID()) {
        CGPoint p;
        if (AXValueGetValue(value, kAXValueCGPointType, &p)) {
            *x_out = p.x;
            *y_out = p.y;
            rc = 0;
        }
    }
    CFRelease(value);
    return rc;
}

int32_t kinax_element_attr_size(uintptr_t handle, const char *attr,
                                double *w_out, double *h_out) {
    if (!w_out || !h_out) return -1;
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return -1;
    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) return -1;
    int32_t rc = -1;
    if (CFGetTypeID(value) == AXValueGetTypeID()) {
        CGSize s;
        if (AXValueGetValue(value, kAXValueCGSizeType, &s)) {
            *w_out = s.width;
            *h_out = s.height;
            rc = 0;
        }
    }
    CFRelease(value);
    return rc;
}

int32_t kinax_element_attr_element_array(uintptr_t handle, const char *attr,
                                         uintptr_t *handles, int32_t *count) {
    if (!count) return -1;
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return -1;
    CFTypeRef value = NULL;
    AXError err = AXUIElementCopyAttributeValue(el, (__bridge CFStringRef)name, &value);
    if (err != kAXErrorSuccess || !value) {
        *count = 0;
        return -1;
    }
    int32_t rc = -1;
    if (CFGetTypeID(value) == CFArrayGetTypeID()) {
        CFArrayRef arr = (CFArrayRef)value;
        CFIndex n = CFArrayGetCount(arr);
        int32_t cap = *count;
        *count = (int32_t)n;
        int32_t fill = (cap < (int32_t)n) ? cap : (int32_t)n;
        for (int32_t i = 0; i < fill; i++) {
            const void *item = CFArrayGetValueAtIndex(arr, i);
            if (item && CFGetTypeID(item) == AXUIElementGetTypeID()) {
                CFRetain(item);
                handles[i] = (uintptr_t)item;
            } else {
                handles[i] = 0;
            }
        }
        rc = 0;
    }
    CFRelease(value);
    return rc;
}

int32_t kinax_element_attribute_names(uintptr_t handle, char *buf, int32_t buflen) {
    AXUIElementRef el = import_element(handle);
    if (!el || !buf || buflen <= 0) return -1;
    CFArrayRef arr = NULL;
    AXError err = AXUIElementCopyAttributeNames(el, &arr);
    if (err != kAXErrorSuccess || !arr) return -1;
    NSArray<NSString *> *names = (__bridge NSArray *)arr;
    NSString *json = json_string_array(names);
    int32_t rc = write_string_result(json, buf, buflen);
    CFRelease(arr);
    return rc;
}

int32_t kinax_element_attr_many(uintptr_t handle, const char *attrs_json,
                                char *buf, int32_t buflen) {
    AXUIElementRef el = import_element(handle);
    if (!el || !attrs_json || !buf || buflen <= 0) return -1;

    // Parse the request: JSON array of strings.
    NSData *jdata = [NSData dataWithBytes:attrs_json length:strlen(attrs_json)];
    NSError *jerr = nil;
    id parsed = [NSJSONSerialization JSONObjectWithData:jdata options:0 error:&jerr];
    if (jerr || ![parsed isKindOfClass:[NSArray class]]) return -1;
    NSArray *names = (NSArray *)parsed;
    if (names.count == 0) {
        return write_string_result(@"{}", buf, buflen);
    }
    for (id n in names) {
        if (![n isKindOfClass:[NSString class]]) return -1;
    }

    // Single AX IPC: fetch all requested attributes at once. Without
    // StopOnError, missing attributes come back as AXValueErrors entries
    // we filter out below.
    CFArrayRef values = NULL;
    AXError err = AXUIElementCopyMultipleAttributeValues(
        el,
        (__bridge CFArrayRef)names,
        0,  // no AXCopyMultipleAttributeOptions
        &values);
    if (err != kAXErrorSuccess || !values) return -1;

    NSMutableDictionary *result = [NSMutableDictionary dictionaryWithCapacity:names.count];
    CFIndex n = CFArrayGetCount(values);
    CFIndex bound = (n < (CFIndex)names.count) ? n : (CFIndex)names.count;

    for (CFIndex i = 0; i < bound; i++) {
        const void *v = CFArrayGetValueAtIndex(values, i);
        if (!v) continue;
        // CFNull marks "no value" in some macOS versions; skip.
        if (v == kCFNull) continue;

        CFTypeID tid = CFGetTypeID(v);

        // AXValue with kAXValueAXErrorType marks missing/unsupported
        // attribute (the multi-fetch convention). Skip.
        if (tid == AXValueGetTypeID()) {
            AXValueType vt = AXValueGetType((AXValueRef)v);
            if (vt == kAXValueAXErrorType) continue;
            // Otherwise it's a real point/size/range/rect — fall through
            // to the description path below (caller should use the
            // dedicated typed entry points for structured access).
        }

        // Element-valued attribute: skip in this batch (caller has the
        // dedicated attr_element / attr_element_array path).
        if (tid == AXUIElementGetTypeID()) continue;
        // Array-of-elements attribute: same.
        if (tid == CFArrayGetTypeID()) continue;

        id key = names[(NSUInteger)i];
        if (tid == CFStringGetTypeID()) {
            result[key] = (__bridge NSString *)v;
        } else if (tid == CFNumberGetTypeID()) {
            // Try to preserve numeric type; fall back to stringValue for
            // unusual number types.
            CFNumberRef num = (CFNumberRef)v;
            if (CFNumberIsFloatType(num)) {
                double d = 0;
                if (CFNumberGetValue(num, kCFNumberDoubleType, &d)) {
                    result[key] = @(d);
                }
            } else {
                long long ll = 0;
                if (CFNumberGetValue(num, kCFNumberLongLongType, &ll)) {
                    result[key] = @(ll);
                }
            }
        } else if (tid == CFBooleanGetTypeID()) {
            result[key] = CFBooleanGetValue(v) ? @YES : @NO;
        } else if (tid == AXValueGetTypeID()) {
            // Stringify CGPoint / CGSize / CGRect / CFRange via description.
            // Callers wanting structured access use the dedicated typed paths.
            result[key] = [(__bridge id)v description];
        } else {
            // Unknown CF type — best-effort description.
            result[key] = [(__bridge id)v description];
        }
    }
    CFRelease(values);

    NSError *enc = nil;
    NSData *out = [NSJSONSerialization dataWithJSONObject:result options:0 error:&enc];
    if (enc || !out) return -1;
    NSString *json = [[NSString alloc] initWithData:out encoding:NSUTF8StringEncoding];
    return write_string_result(json, buf, buflen);
}

int32_t kinax_element_action_names(uintptr_t handle, char *buf, int32_t buflen) {
    AXUIElementRef el = import_element(handle);
    if (!el || !buf || buflen <= 0) return -1;
    CFArrayRef arr = NULL;
    AXError err = AXUIElementCopyActionNames(el, &arr);
    if (err != kAXErrorSuccess || !arr) return -1;
    NSArray<NSString *> *names = (__bridge NSArray *)arr;
    NSString *json = json_string_array(names);
    int32_t rc = write_string_result(json, buf, buflen);
    CFRelease(arr);
    return rc;
}

int32_t kinax_element_perform(uintptr_t handle, const char *action) {
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(action);
    if (!el || !name) return -1;
    AXError err = AXUIElementPerformAction(el, (__bridge CFStringRef)name);
    return (err == kAXErrorSuccess) ? 0 : (int32_t)err;
}

int32_t kinax_element_set_string(uintptr_t handle, const char *attr,
                                 const char *value) {
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    NSString *val  = nsstr(value);
    if (!el || !name || !val) return -1;
    AXError err = AXUIElementSetAttributeValue(
        el, (__bridge CFStringRef)name, (__bridge CFTypeRef)val);
    return (err == kAXErrorSuccess) ? 0 : (int32_t)err;
}

int32_t kinax_element_set_bool(uintptr_t handle, const char *attr, int32_t value) {
    AXUIElementRef el = import_element(handle);
    NSString *name = nsstr(attr);
    if (!el || !name) return -1;
    CFBooleanRef b = value ? kCFBooleanTrue : kCFBooleanFalse;
    AXError err = AXUIElementSetAttributeValue(
        el, (__bridge CFStringRef)name, b);
    return (err == kAXErrorSuccess) ? 0 : (int32_t)err;
}
