<?php
/**
 * Plugin Name:       wpresto Go Live
 * Description:       Triggers a "go live" static rebuild of this WordPress site by calling the localhost hook endpoint served by the wpresto Go binary.
 * Version:           1.0.0
 * Author:            t3km4n
 * License:           GPL-2.0-or-later
 * License URI:       https://www.gnu.org/licenses/gpl-2.0.html
 * Text Domain:       wpresto-golive
 * Domain Path:       /languages
 * Requires at least: 6.0
 * Requires PHP:      7.4
 *
 * Hook endpoint contract (served by the wpresto Go binary):
 *   POST {base}/go-live  with header X-WPresto-Secret: <secret>
 *     202 {"build_id":"<hex>","status":"running"}   build accepted
 *     409 {"error":"build already running"}         build in flight
 *     401 {"error":"unauthorized"}                  bad/missing secret
 *   GET  {base}/status  (no auth)
 *     {"build_id":"...","status":"idle|running|success|error","error":"..."}
 *
 * @package WprestoGolive
 */

if ( ! defined( 'ABSPATH' ) ) {
	exit;
}

/**
 * Plugin version, used for cache busting of inline scripts/styles.
 */
define( 'WPRESTO_GOLIVE_VERSION', '1.0.0' );

/**
 * Default base URL of the wpresto hook endpoint (no trailing slash).
 */
define( 'WPRESTO_GOLIVE_DEFAULT_HOOK_URL', 'http://127.0.0.1:9090' );

/*
 * ---------------------------------------------------------------------------
 * Option accessors
 * ---------------------------------------------------------------------------
 */

/**
 * Get the configured hook base URL, normalized (trimmed, no trailing slash).
 *
 * @return string
 */
function wpresto_golive_get_hook_url() {
	$url = get_option( 'wpresto_hook_url', WPRESTO_GOLIVE_DEFAULT_HOOK_URL );

	if ( ! is_string( $url ) || '' === trim( $url ) ) {
		return WPRESTO_GOLIVE_DEFAULT_HOOK_URL;
	}

	return untrailingslashit( trim( $url ) );
}

/**
 * Get the configured hook secret.
 *
 * @return string
 */
function wpresto_golive_get_hook_secret() {
	$secret = get_option( 'wpresto_hook_secret', '' );

	return is_string( $secret ) ? $secret : '';
}

/*
 * ---------------------------------------------------------------------------
 * Settings page
 * ---------------------------------------------------------------------------
 */

/**
 * Sanitize the hook URL option.
 *
 * @param mixed $value Raw value from the settings form.
 * @return string
 */
function wpresto_golive_sanitize_hook_url( $value ) {
	$value = esc_url_raw( trim( (string) $value ) );

	return '' === $value ? WPRESTO_GOLIVE_DEFAULT_HOOK_URL : $value;
}

/**
 * Sanitize the hook secret option.
 *
 * @param mixed $value Raw value from the settings form.
 * @return string
 */
function wpresto_golive_sanitize_hook_secret( $value ) {
	return sanitize_text_field( (string) $value );
}

/**
 * Register the settings page under the Settings menu.
 *
 * @return void
 */
function wpresto_golive_register_settings_page() {
	add_options_page(
		__( 'wpresto Go Live', 'wpresto-golive' ),
		__( 'wpresto Go Live', 'wpresto-golive' ),
		'manage_options',
		'wpresto-golive',
		'wpresto_golive_render_settings_page'
	);
}
add_action( 'admin_menu', 'wpresto_golive_register_settings_page' );

/**
 * Register the settings, section, and fields.
 *
 * @return void
 */
function wpresto_golive_register_settings() {
	register_setting(
		'wpresto_golive_settings',
		'wpresto_hook_url',
		array(
			'type'              => 'string',
			'default'           => WPRESTO_GOLIVE_DEFAULT_HOOK_URL,
			'sanitize_callback' => 'wpresto_golive_sanitize_hook_url',
		)
	);

	register_setting(
		'wpresto_golive_settings',
		'wpresto_hook_secret',
		array(
			'type'              => 'string',
			'default'           => '',
			'sanitize_callback' => 'wpresto_golive_sanitize_hook_secret',
		)
	);

	add_settings_section(
		'wpresto_golive_endpoint',
		__( 'Hook endpoint', 'wpresto-golive' ),
		'wpresto_golive_section_callback',
		'wpresto-golive'
	);

	add_settings_field(
		'wpresto_hook_url',
		__( 'Hook URL', 'wpresto-golive' ),
		'wpresto_golive_field_hook_url',
		'wpresto-golive',
		'wpresto_golive_endpoint'
	);

	add_settings_field(
		'wpresto_hook_secret',
		__( 'Hook secret', 'wpresto-golive' ),
		'wpresto_golive_field_hook_secret',
		'wpresto-golive',
		'wpresto_golive_endpoint'
	);
}
add_action( 'admin_init', 'wpresto_golive_register_settings' );

/**
 * Render the settings section description.
 *
 * @return void
 */
function wpresto_golive_section_callback() {
	echo '<p>' . esc_html__(
		'The wpresto Go binary listens on this host and rebuilds the static site when a "go live" trigger is sent to its /go-live endpoint.',
		'wpresto-golive'
	) . '</p>';
}

/**
 * Render the Hook URL field.
 *
 * @return void
 */
function wpresto_golive_field_hook_url() {
	printf(
		'<input type="url" class="regular-text" id="wpresto_hook_url" name="wpresto_hook_url" value="%s" placeholder="%s">',
		esc_url( wpresto_golive_get_hook_url() ),
		esc_attr( WPRESTO_GOLIVE_DEFAULT_HOOK_URL )
	);
}

/**
 * Render the Hook secret field.
 *
 * @return void
 */
function wpresto_golive_field_hook_secret() {
	printf(
		'<input type="password" class="regular-text" id="wpresto_hook_secret" name="wpresto_hook_secret" value="%s" autocomplete="new-password">',
		esc_attr( wpresto_golive_get_hook_secret() )
	);
}

/**
 * Render the settings page.
 *
 * @return void
 */
function wpresto_golive_render_settings_page() {
	if ( ! current_user_can( 'manage_options' ) ) {
		return;
	}
	?>
	<div class="wrap">
		<h1 class="wp-heading-inline"><?php esc_html_e( 'wpresto Go Live', 'wpresto-golive' ); ?></h1>
		<hr class="wp-header-end">

		<p><?php esc_html_e( 'Configure the localhost hook endpoint served by the wpresto Go binary, then trigger a static rebuild of this site.', 'wpresto-golive' ); ?></p>

		<form action="options.php" method="post">
			<?php
			settings_fields( 'wpresto_golive_settings' );
			do_settings_sections( 'wpresto-golive' );
			submit_button();
			?>
		</form>

		<hr>

		<p>
			<button type="button" class="button button-primary" id="wpresto-golive-now"><?php esc_html_e( 'Go Live Now', 'wpresto-golive' ); ?></button>
			<span id="wpresto-golive-status" class="wpresto-golive-status" role="status" aria-live="polite"></span>
		</p>
	</div>
	<?php
}

/*
 * ---------------------------------------------------------------------------
 * AJAX trigger
 * ---------------------------------------------------------------------------
 */

/**
 * Handle the "go live" AJAX request.
 *
 * Auth: logged-in users with manage_options + a valid wp_ajax nonce.
 * The hook URL and secret are read from the options; the values posted by
 * the client are intentionally ignored — the server is the source of truth.
 *
 * @return void
 */
function wpresto_golive_ajax_trigger() {
	check_ajax_referer( 'wp_ajax', 'nonce' );

	if ( ! current_user_can( 'manage_options' ) ) {
		wp_send_json_error( __( 'You are not allowed to trigger a rebuild.', 'wpresto-golive' ), 403 );
	}

	$hook_url = wpresto_golive_get_hook_url();
	$secret   = wpresto_golive_get_hook_secret();

	if ( '' === $secret ) {
		wp_send_json_error(
			__( 'No hook secret configured. Set it on the "wpresto Go Live" settings page first.', 'wpresto-golive' ),
			400
		);
	}

	$response = wp_remote_post(
		$hook_url . '/go-live',
		array(
			'timeout' => 10,
			'headers' => array(
				'X-WPresto-Secret' => $secret,
				'Content-Type'      => 'application/json',
			),
			'body'    => '{}',
		)
	);

	if ( is_wp_error( $response ) ) {
		wp_send_json_error(
			sprintf(
				/* translators: %s: underlying HTTP error message. */
				__( 'Could not reach the wpresto hook: %s', 'wpresto-golive' ),
				$response->get_error_message()
			),
			502
		);
	}

	$code = (int) wp_remote_retrieve_response_code( $response );
	$data = json_decode( (string) wp_remote_retrieve_body( $response ), true );
	$data = is_array( $data ) ? $data : array();

	switch ( $code ) {
		case 202:
			wp_send_json_success(
				array(
					'build_id' => isset( $data['build_id'] ) ? (string) $data['build_id'] : '',
					'status'   => isset( $data['status'] ) ? (string) $data['status'] : 'running',
				)
			);
			break;

		case 409:
			wp_send_json_error( __( 'A build is already running.', 'wpresto-golive' ), 409 );
			break;

		case 401:
			wp_send_json_error( __( 'Unauthorized (check the secret).', 'wpresto-golive' ), 401 );
			break;

		default:
			wp_send_json_error(
				sprintf(
					/* translators: %d: HTTP status code returned by the hook. */
					__( 'The wpresto hook returned HTTP %d.', 'wpresto-golive' ),
					$code
				),
				$code
			);
	}
}
add_action( 'wp_ajax_wpresto_golive', 'wpresto_golive_ajax_trigger' );

/*
 * ---------------------------------------------------------------------------
 * Admin bar
 * ---------------------------------------------------------------------------
 */

/**
 * Add the "Go Live" node to the admin bar (admins only).
 *
 * The node's href is a no-op anchor; the inline script intercepts the click
 * and fires the AJAX trigger instead.
 *
 * @param WP_Admin_Bar $wp_admin_bar Admin bar instance.
 * @return void
 */
function wpresto_golive_admin_bar_node( $wp_admin_bar ) {
	if ( ! current_user_can( 'manage_options' ) ) {
		return;
	}

	$wp_admin_bar->add_node(
		array(
			'id'    => 'wpresto-golive',
			'title' => __( 'Go Live', 'wpresto-golive' ),
			'href'  => '#wpresto-golive',
		)
	);
}
add_action( 'admin_bar_menu', 'wpresto_golive_admin_bar_node', 999 );

/*
 * ---------------------------------------------------------------------------
 * Front-end / admin assets
 * ---------------------------------------------------------------------------
 */

/**
 * Inline trigger script, bound to the settings page button and the admin bar node.
 *
 * @return string
 */
function wpresto_golive_inline_js() {
	return <<<'JS'
( function () {
	'use strict';

	if ( typeof window.wprestoGolive === 'undefined' ) {
		return;
	}

	var cfg = window.wprestoGolive;

	/**
	 * Find (or create, next to the given anchor) the status element.
	 *
	 * @param {HTMLElement} anchor Element the status should appear next to.
	 * @returns {HTMLElement|null}
	 */
	function statusElement( anchor ) {
		var el = document.getElementById( 'wpresto-golive-status' );

		if ( ! el && anchor && anchor.parentNode ) {
			el = document.createElement( 'span' );
			el.id = 'wpresto-golive-status';
			el.className = 'wpresto-golive-status';
			anchor.parentNode.insertBefore( el, anchor.nextSibling );
		}

		return el;
	}

	/**
	 * Show a transient status message.
	 *
	 * @param {string} text Message text.
	 * @param {string} type One of "working", "success", "error".
	 * @param {HTMLElement} anchor Anchor element.
	 */
	function showStatus( text, type, anchor ) {
		var el = statusElement( anchor );
		if ( ! el ) {
			return;
		}
		el.textContent = text;
		el.className = 'wpresto-golive-status wpresto-golive-status--' + type;
	}

	/**
	 * Fire the go-live AJAX request.
	 *
	 * @param {HTMLElement} anchor Anchor element for the status message.
	 */
	function trigger( anchor ) {
		var body = new URLSearchParams();
		body.append( 'action', 'wpresto_golive' );
		body.append( 'nonce', cfg.nonce );
		body.append( 'hook_url', cfg.hookUrl );
		body.append( 'secret', cfg.secret );

		showStatus( cfg.i18n.working, 'working', anchor );

		fetch( cfg.ajaxUrl, {
			method: 'POST',
			credentials: 'same-origin',
			body: body
		} )
			.then( function ( response ) {
				return response.json();
			} )
			.then( function ( json ) {
				if ( json && json.success ) {
					var buildId = json.data && json.data.build_id ? json.data.build_id : '';
					showStatus(
						cfg.i18n.success + ( buildId ? ' (' + buildId + ')' : '' ),
						'success',
						anchor
					);
					return;
				}

				var message = ( json && json.data && typeof json.data === 'string' ) ? json.data : cfg.i18n.error;
				showStatus( cfg.i18n.error + ': ' + message, 'error', anchor );
			} )
			.catch( function () {
				showStatus( cfg.i18n.error, 'error', anchor );
			} );
	}

	// Settings page "Go Live Now" button.
	var settingsButton = document.getElementById( 'wpresto-golive-now' );
	if ( settingsButton ) {
		settingsButton.addEventListener( 'click', function ( event ) {
			event.preventDefault();
			trigger( settingsButton );
		} );
	}

	// Admin bar "Go Live" node.
	var adminBarLink = document.querySelector( '#ab-top-menu-wpresto-golive a' );
	if ( adminBarLink ) {
		adminBarLink.addEventListener( 'click', function ( event ) {
			event.preventDefault();
			trigger( adminBarLink );
		} );
	}
} )();
JS;
}

/**
 * Enqueue the trigger script + localized data.
 *
 * Only for users with manage_options: the localized object carries the hook
 * secret, and only those users get the UI (admin bar node / settings button)
 * that consumes it.
 *
 * @return void
 */
function wpresto_golive_enqueue_scripts() {
	if ( ! current_user_can( 'manage_options' ) ) {
		return;
	}

	// Inline-only script (no external file).
	wp_register_script(
		'wpresto-golive',
		false,
		array(),
		WPRESTO_GOLIVE_VERSION,
		true
	);

	// Localized data: AJAX endpoint, nonce, hook URL + secret, i18n strings.
	wp_localize_script(
		'wpresto-golive',
		'wprestoGolive',
		array(
			'ajaxUrl' => admin_url( 'admin-ajax.php' ),
			'nonce'   => wp_create_nonce( 'wp_ajax' ),
			'hookUrl' => wpresto_golive_get_hook_url(),
			'secret'  => wpresto_golive_get_hook_secret(),
			'i18n'    => array(
				'working' => __( 'Triggering rebuild…', 'wpresto-golive' ),
				'success' => __( 'Rebuild started', 'wpresto-golive' ),
				'error'   => __( 'Go live failed', 'wpresto-golive' ),
			),
		)
	);

	// The trigger script itself.
	wp_add_inline_script( 'wpresto-golive', wpresto_golive_inline_js(), 'after' );

	// Inline-only stylesheet for the status message.
	wp_register_style( 'wpresto-golive', false, array(), WPRESTO_GOLIVE_VERSION );
	wp_add_inline_style(
		'wpresto-golive',
		'.wpresto-golive-status{margin-left:8px;font-weight:600;}' .
		'.wpresto-golive-status--working{color:#646970;}' .
		'.wpresto-golive-status--success{color:#00a32a;}' .
		'.wpresto-golive-status--error{color:#d63638;}'
	);

	wp_enqueue_script( 'wpresto-golive' );
	wp_enqueue_style( 'wpresto-golive' );
}
add_action( 'admin_enqueue_scripts', 'wpresto_golive_enqueue_scripts' );

/**
 * Front end: only load when the admin bar is visible (for the admin bar node).
 *
 * @return void
 */
function wpresto_golive_maybe_enqueue_front_end() {
	if ( is_admin_bar_showing() ) {
		wpresto_golive_enqueue_scripts();
	}
}
add_action( 'wp_enqueue_scripts', 'wpresto_golive_maybe_enqueue_front_end' );
